// Package googlevision provides an ocr.Engine backed by the Google Cloud
// Vision REST API (images:annotate with DOCUMENT_TEXT_DETECTION).
//
// Unlike the Tesseract adapter it needs no CGo and no third-party packages —
// only the standard library — so it lives in the core module. It is the
// recommended engine for KTP and passport accuracy, where local OCR is
// unreliable.
//
// Authentication is either an API key (X-Goog-Api-Key header) or an OAuth2
// bearer token (Authorization header), e.g. the output of
// `gcloud auth application-default print-access-token`. Credentials never
// travel in the URL and never appear in returned errors:
//
//	engine := googlevision.New(googlevision.WithAPIKey(os.Getenv("GOOGLE_VISION_API_KEY")))
//	// or
//	engine := googlevision.New(googlevision.WithBearerToken(token))
package googlevision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/reynaldio/id-ocr/ocr"
)

const defaultEndpoint = "https://vision.googleapis.com/v1/images:annotate"

// Engine is a Google Cloud Vision-backed ocr.Engine.
type Engine struct {
	apiKey      string
	bearerToken string
	endpoint    string
	feature     string
	langHints   []string
	httpClient  *http.Client
}

// Option configures an Engine.
type Option func(*Engine)

// WithAPIKey authenticates requests with a Vision API key.
func WithAPIKey(key string) Option { return func(e *Engine) { e.apiKey = key } }

// WithBearerToken authenticates requests with an OAuth2 access token, sent as
// an Authorization: Bearer header. Use this with service-account / ADC tokens.
func WithBearerToken(token string) Option { return func(e *Engine) { e.bearerToken = token } }

// WithEndpoint overrides the API endpoint (useful for testing or regional
// endpoints).
func WithEndpoint(endpoint string) Option { return func(e *Engine) { e.endpoint = endpoint } }

// WithFeature sets the detection feature. Defaults to DOCUMENT_TEXT_DETECTION
// (dense document text); TEXT_DETECTION suits sparse text.
func WithFeature(feature string) Option { return func(e *Engine) { e.feature = feature } }

// WithLanguageHints biases recognition toward the given BCP-47 languages,
// e.g. "id", "en". Defaults to none (auto-detect).
func WithLanguageHints(langs ...string) Option {
	return func(e *Engine) { e.langHints = langs }
}

// WithHTTPClient sets the HTTP client used for requests. The default client
// refuses redirects so credential headers are never forwarded to another
// host; a custom client's redirect policy is its own.
func WithHTTPClient(c *http.Client) Option { return func(e *Engine) { e.httpClient = c } }

// New returns a Vision engine. Provide WithAPIKey or WithBearerToken for
// authentication.
func New(opts ...Option) *Engine {
	e := &Engine{
		endpoint:   defaultEndpoint,
		feature:    "DOCUMENT_TEXT_DETECTION",
		httpClient: &http.Client{CheckRedirect: noRedirect},
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Recognize implements ocr.Engine by sending img to the Vision API.
func (e *Engine) Recognize(ctx context.Context, img image.Image) (*ocr.Result, error) {
	if e.apiKey == "" && e.bearerToken == "" {
		return nil, fmt.Errorf("googlevision: no credentials (set WithAPIKey or WithBearerToken)")
	}

	var png bytes.Buffer
	if err := encodePNG(&png, img); err != nil {
		return nil, err
	}
	body, err := json.Marshal(e.buildRequest(png.Bytes()))
	if err != nil {
		return nil, err
	}

	req, err := e.newHTTPRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	resp, err := e.httpClient.Do(req)
	if err != nil {
		// *url.Error prints the request URL; keep only the cause (still
		// unwrappable, e.g. to context.DeadlineExceeded).
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, e.redact(fmt.Errorf("googlevision: request failed: %w", err))
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, e.redact(fmt.Errorf("googlevision: read response: %w", err))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, httpError(resp.StatusCode, raw)
	}
	return parseResult(raw)
}

// httpError reports a non-200 response by status code and Google's error
// status (e.g. PERMISSION_DENIED) only. The body is untrusted — a proxy may
// echo the request — so free text from it is never included.
func httpError(code int, raw []byte) error {
	var env struct {
		Error struct {
			Status string `json:"status"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &env) == nil && isStatusCode(env.Error.Status) {
		return fmt.Errorf("googlevision: HTTP %d %s", code, env.Error.Status)
	}
	return fmt.Errorf("googlevision: HTTP %d", code)
}

// isStatusCode reports whether s looks like a google.rpc.Code name
// (UPPER_SNAKE_CASE), so arbitrary text can't ride along in the status field.
func isStatusCode(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if (r < 'A' || r > 'Z') && r != '_' {
			return false
		}
	}
	return true
}

// redact is a last line of defence: it blanks credentials from err's text
// while keeping err unwrappable.
func (e *Engine) redact(err error) error {
	msg := err.Error()
	clean := msg
	for _, secret := range []string{e.apiKey, e.bearerToken} {
		if secret != "" {
			clean = strings.ReplaceAll(clean, secret, "[REDACTED]")
		}
	}
	if clean == msg {
		return err
	}
	return &redactedError{msg: clean, err: err}
}

type redactedError struct {
	msg string
	err error
}

func (r *redactedError) Error() string { return r.msg }
func (r *redactedError) Unwrap() error { return r.err }

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

func (e *Engine) buildRequest(imgData []byte) annotateRequest {
	r := annotateRequest{Requests: []annotateImageRequest{{
		Image:    imageData{Content: base64.StdEncoding.EncodeToString(imgData)},
		Features: []feature{{Type: e.feature}},
	}}}
	if len(e.langHints) > 0 {
		r.Requests[0].ImageContext = &imageContext{LanguageHints: e.langHints}
	}
	return r
}

func (e *Engine) newHTTPRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("X-Goog-Api-Key", e.apiKey)
	}
	if e.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+e.bearerToken)
	}
	return req, nil
}

func encodePNG(buf *bytes.Buffer, img image.Image) error {
	if err := png.Encode(buf, img); err != nil {
		return fmt.Errorf("googlevision: encode image: %w", err)
	}
	return nil
}

// Package googlevision provides an ocr.Engine backed by the Google Cloud
// Vision REST API (images:annotate with DOCUMENT_TEXT_DETECTION).
//
// Unlike the Tesseract adapter it needs no CGo and no third-party packages —
// only the standard library — so it lives in the core module. It is the
// recommended engine for KTP and passport accuracy, where local OCR is
// unreliable.
//
// Authentication is either an API key (query parameter) or an OAuth2 bearer
// token (Authorization header), e.g. the output of
// `gcloud auth application-default print-access-token`:
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
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"

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

// WithHTTPClient sets the HTTP client used for requests.
func WithHTTPClient(c *http.Client) Option { return func(e *Engine) { e.httpClient = c } }

// New returns a Vision engine. Provide WithAPIKey or WithBearerToken for
// authentication.
func New(opts ...Option) *Engine {
	e := &Engine{
		endpoint:   defaultEndpoint,
		feature:    "DOCUMENT_TEXT_DETECTION",
		httpClient: http.DefaultClient,
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
		return nil, fmt.Errorf("googlevision: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("googlevision: HTTP %d: %s", resp.StatusCode, truncate(raw))
	}
	return parseResult(raw)
}

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
	endpoint := e.endpoint
	if e.apiKey != "" {
		endpoint += "?key=" + url.QueryEscape(e.apiKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
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

func truncate(b []byte) string {
	const max = 512
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}

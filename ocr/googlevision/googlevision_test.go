package googlevision

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/reynaldio/id-ocr/ocr"
)

func blankImage() image.Image { return image.NewRGBA(image.Rect(0, 0, 4, 4)) }

func TestRecognize_ParsesTextAndWords(t *testing.T) {
	// textAnnotations[0] is the whole text; the rest are positioned words. The
	// word order here is deliberately scrambled (as Vision does for forms) to
	// prove geometry reconstruction re-rows them.
	const respBody = `{
	  "responses": [{
	    "fullTextAnnotation": {"text": "ignored when words have geometry"},
	    "textAnnotations": [
	      {"description": "PROVINSI NIK 3171010905900001"},
	      {"description": "NIK",      "boundingPoly": {"vertices": [{"x":10,"y":60},{"x":60,"y":60},{"x":60,"y":80},{"x":10,"y":80}]}},
	      {"description": "PROVINSI", "boundingPoly": {"vertices": [{"x":10,"y":20},{"x":90,"y":20},{"x":90,"y":40},{"x":10,"y":40}]}},
	      {"description": "3171010905900001", "boundingPoly": {"vertices": [{"x":120,"y":60},{"x":300,"y":60},{"x":300,"y":80},{"x":120,"y":80}]}}
	    ]
	  }]
	}`

	var gotReq annotateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Goog-Api-Key"); got != "secret" {
			t.Errorf("X-Goog-Api-Key = %q, want secret", got)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty (key must not travel in the URL)", r.URL.RawQuery)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		io.WriteString(w, respBody)
	}))
	defer srv.Close()

	eng := New(WithAPIKey("secret"), WithEndpoint(srv.URL), WithLanguageHints("id", "en"))
	res, err := eng.Recognize(context.Background(), blankImage())
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}

	// Request was well-formed.
	if len(gotReq.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(gotReq.Requests))
	}
	if gotReq.Requests[0].Features[0].Type != "DOCUMENT_TEXT_DETECTION" {
		t.Errorf("feature = %q", gotReq.Requests[0].Features[0].Type)
	}
	if gotReq.Requests[0].Image.Content == "" {
		t.Error("image content was empty")
	}
	if gotReq.Requests[0].ImageContext == nil ||
		strings.Join(gotReq.Requests[0].ImageContext.LanguageHints, ",") != "id,en" {
		t.Errorf("language hints = %+v", gotReq.Requests[0].ImageContext)
	}

	// Response parsed, with geometry reconstruction re-rowing the words so the
	// NIK label and value share a line.
	if res.Text != "PROVINSI\nNIK 3171010905900001" {
		t.Errorf("Text = %q", res.Text)
	}
	if len(res.Words) != 3 {
		t.Fatalf("Words = %+v", res.Words)
	}
	var provinsi *ocr.Word
	for i := range res.Words {
		if res.Words[i].Text == "PROVINSI" {
			provinsi = &res.Words[i]
		}
	}
	if provinsi == nil {
		t.Fatal("PROVINSI word not found")
	}
	if got := provinsi.Box; got.Min.X != 10 || got.Max.X != 90 || got.Max.Y != 40 {
		t.Errorf("PROVINSI box = %v", got)
	}
}

func TestRecognize_BearerTokenAndAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		io.WriteString(w, `{"responses":[{"error":{"code":7,"message":"PERMISSION_DENIED"}}]}`)
	}))
	defer srv.Close()

	eng := New(WithBearerToken("tok"), WithEndpoint(srv.URL))
	_, err := eng.Recognize(context.Background(), blankImage())
	if err == nil || !strings.Contains(err.Error(), "PERMISSION_DENIED") {
		t.Fatalf("err = %v, want PERMISSION_DENIED", err)
	}
}

func TestRecognize_NoCredentials(t *testing.T) {
	_, err := New().Recognize(context.Background(), blankImage())
	if err == nil || !strings.Contains(err.Error(), "no credentials") {
		t.Fatalf("err = %v, want no-credentials error", err)
	}
}

// The key must never surface in error text: consumers log and return these.
const leakKey = "AIzaSy-super-secret-key"

func TestRecognize_HTTPErrorHidesBodyAndKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A misbehaving proxy echoing the request back.
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"error":{"code":403,"message":"echo `+r.URL.String()+` `+r.Header.Get("X-Goog-Api-Key")+`","status":"PERMISSION_DENIED"}}`)
	}))
	defer srv.Close()

	_, err := New(WithAPIKey(leakKey), WithEndpoint(srv.URL)).Recognize(context.Background(), blankImage())
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "403") || !strings.Contains(msg, "PERMISSION_DENIED") {
		t.Errorf("err = %q, want HTTP 403 PERMISSION_DENIED", msg)
	}
	if strings.Contains(msg, leakKey) || strings.Contains(msg, "echo") {
		t.Errorf("err leaks response body or key: %q", msg)
	}
}

func TestRecognize_HTTPErrorUnparsableBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, "<html>gateway saw key="+leakKey+"</html>")
	}))
	defer srv.Close()

	_, err := New(WithAPIKey(leakKey), WithEndpoint(srv.URL)).Recognize(context.Background(), blankImage())
	if err == nil || err.Error() != "googlevision: HTTP 502" {
		t.Fatalf("err = %v, want googlevision: HTTP 502", err)
	}
}

func TestRecognize_TransportErrorHidesKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := srv.URL
	srv.Close() // connection refused from here on

	_, err := New(WithAPIKey(leakKey), WithEndpoint(endpoint)).Recognize(context.Background(), blankImage())
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), leakKey) {
		t.Errorf("err leaks key: %q", err)
	}
}

func TestRecognize_TimeoutKeepsContextErrorAndHidesKey(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := New(WithAPIKey(leakKey), WithEndpoint(srv.URL)).Recognize(ctx, blankImage())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if strings.Contains(err.Error(), leakKey) {
		t.Errorf("err leaks key: %q", err)
	}
}

func TestRecognize_DoesNotFollowRedirects(t *testing.T) {
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("X-Goog-Api-Key") != ""
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	_, err := New(WithAPIKey(leakKey), WithEndpoint(srv.URL)).Recognize(context.Background(), blankImage())
	if err == nil {
		t.Fatal("want error for a redirect response")
	}
	if leaked {
		t.Error("API key header was forwarded to the redirect target")
	}
}

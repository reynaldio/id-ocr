package googlevision

import (
	"context"
	"encoding/json"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		if got := r.URL.Query().Get("key"); got != "secret" {
			t.Errorf("api key = %q, want secret", got)
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

package idocr

import (
	"context"
	"image"
	"testing"

	"github.com/reynaldio/id-ocr/ocr"
)

// stubEngine is a fixed-text ocr.Engine for testing the Client wiring without
// any real OCR backend. id identifies which engine handled a call in routing
// tests.
type stubEngine struct {
	id   string
	text string
}

func (s stubEngine) Recognize(context.Context, image.Image) (*ocr.Result, error) {
	return &ocr.Result{Text: s.text}, nil
}

func TestNewAuto_Routing(t *testing.T) {
	cloud := stubEngine{id: "cloud"}
	local := stubEngine{id: "local"}

	// Both engines: KTP/passport -> cloud, NPWP -> local.
	both := NewAuto(cloud, local)
	assertEngine(t, both, DocKTP, "cloud")
	assertEngine(t, both, DocPassport, "cloud")
	assertEngine(t, both, DocNPWP, "local")

	// No cloud (no Vision API key): everything falls back to Tesseract.
	noCloud := NewAuto(nil, local)
	assertEngine(t, noCloud, DocKTP, "local")
	assertEngine(t, noCloud, DocPassport, "local")
	assertEngine(t, noCloud, DocNPWP, "local")

	// No local: everything uses cloud.
	noLocal := NewAuto(cloud, nil)
	assertEngine(t, noLocal, DocKTP, "cloud")
	assertEngine(t, noLocal, DocNPWP, "cloud")
}

func TestWithEngineFor(t *testing.T) {
	c := New(stubEngine{id: "default"}, WithEngineFor(DocNPWP, stubEngine{id: "npwp"}))
	assertEngine(t, c, DocKTP, "default")
	assertEngine(t, c, DocNPWP, "npwp")
}

func TestEngineFor_NoneConfigured(t *testing.T) {
	if _, err := NewAuto(nil, nil).engineFor(DocKTP); err == nil {
		t.Error("expected an error when no engine is configured")
	}
}

func assertEngine(t *testing.T, c *Client, dt DocumentType, wantID string) {
	t.Helper()
	e, err := c.engineFor(dt)
	if err != nil {
		t.Fatalf("engineFor(%s): %v", dt, err)
	}
	if got := e.(stubEngine).id; got != wantID {
		t.Errorf("%s routed to %q, want %q", dt, got, wantID)
	}
}

func TestClient_RecognizeKTP(t *testing.T) {
	c := New(stubEngine{text: "NIK : 3171010905900001\nNama : BUDI SANTOSO"})
	k, err := c.RecognizeKTP(context.Background(), image.NewRGBA(image.Rect(0, 0, 1, 1)))
	if err != nil {
		t.Fatalf("RecognizeKTP: %v", err)
	}
	if k.NIK != "3171010905900001" {
		t.Errorf("NIK = %q", k.NIK)
	}
	if k.Name != "BUDI SANTOSO" {
		t.Errorf("Name = %q", k.Name)
	}
}

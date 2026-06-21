// Package ocr defines the pluggable text-recognition contract used by the
// id-ocr document parsers. The core library depends only on this interface,
// so callers can plug in any backend (Tesseract, Google Vision, AWS Textract,
// a test stub, ...) without pulling heavy or CGo dependencies into the parsers.
package ocr

import (
	"context"
	"image"
	"strings"
)

// Engine extracts text from an image. Implementations live in their own
// subpackages (e.g. ocr/tesseract) or in the caller's code.
type Engine interface {
	// Recognize runs OCR on img and returns the detected text. It must
	// honour ctx for cancellation and deadlines.
	Recognize(ctx context.Context, img image.Image) (*Result, error)
}

// Result is the raw output of an OCR engine. Word boxes are optional: engines
// that cannot supply per-word geometry or confidence may leave Words nil and
// populate only Text.
type Result struct {
	// Text is the full recognised text, newline-separated by line.
	Text string
	// Words holds optional per-word detail (geometry + confidence).
	Words []Word
}

// Word is a single recognised token with optional layout information.
type Word struct {
	Text       string
	Confidence float64 // 0..100, engine-defined; 0 if unknown
	Box        image.Rectangle
}

// Lines splits Text into trimmed, non-empty lines. This is the primary input
// the document parsers work from.
func (r *Result) Lines() []string {
	if r == nil {
		return nil
	}
	raw := strings.Split(r.Text, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

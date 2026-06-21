// Package tesseract provides an ocr.Engine backed by Tesseract via the
// gosseract CGo binding.
//
// It lives in its own nested Go module so its CGo + native Tesseract
// dependency stays out of the core id-ocr module: `go get` and
// `go mod tidy` on github.com/reynaldio/id-ocr never pull it in. Consumers
// who want it import this package's module explicitly.
//
// It requires CGo and a system Tesseract install (libtesseract + language
// data). On macOS: `brew install tesseract`. On Debian/Ubuntu:
// `apt-get install tesseract-ocr libtesseract-dev`. Add the Indonesian model
// (`tesseract-ocr-ind`) for best KTP/NPWP results.
package tesseract

import (
	"bytes"
	"context"
	"image"
	"image/png"

	"github.com/otiai10/gosseract/v2"

	"github.com/reynaldio/id-ocr/ocr"
	"github.com/reynaldio/id-ocr/preprocess"
)

// defaultScale is the upscale factor of the default preprocessor; small ID-card
// fonts OCR much better enlarged.
const defaultScale = 2.5

// Engine is a Tesseract-backed ocr.Engine.
type Engine struct {
	languages []string
	psm       int                           // page-segmentation mode; -1 = engine default
	prep      func(image.Image) image.Image // optional preprocessing hook
}

// Option configures an Engine.
type Option func(*Engine)

// WithLanguages sets the Tesseract language list (e.g. "ind", "eng").
// Defaults to {"ind", "eng"}.
func WithLanguages(langs ...string) Option {
	return func(e *Engine) { e.languages = langs }
}

// WithPageSegMode overrides the Tesseract page-segmentation mode (PSM). The
// default is 6 ("assume a single uniform block of text"), which works well for
// ID cards. Pass -1 to leave Tesseract's own default. See gosseract.PSM_*.
func WithPageSegMode(psm int) Option {
	return func(e *Engine) { e.psm = psm }
}

// WithPreprocessor overrides the image transform applied before OCR. The
// default is preprocess.Document (grayscale + upscale), which greatly improves
// accuracy on photographed cards. Pass nil to disable preprocessing.
func WithPreprocessor(fn func(image.Image) image.Image) Option {
	return func(e *Engine) { e.prep = fn }
}

// New returns a Tesseract engine ready to use with no configuration: Indonesian
// + English languages, page-segmentation mode 6, and grayscale + 2.5x upscale
// preprocessing — the settings that read Indonesian ID cards well. Override any
// of these with the With* options.
func New(opts ...Option) *Engine {
	e := &Engine{
		languages: []string{"ind", "eng"},
		psm:       6,
		prep:      func(im image.Image) image.Image { return preprocess.Document(im, defaultScale) },
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Recognize implements ocr.Engine.
func (e *Engine) Recognize(ctx context.Context, img image.Image) (*ocr.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e.prep != nil {
		img = e.prep(img)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}

	client := gosseract.NewClient()
	defer client.Close()
	if err := client.SetLanguage(e.languages...); err != nil {
		return nil, err
	}
	if e.psm >= 0 {
		if err := client.SetPageSegMode(gosseract.PageSegMode(e.psm)); err != nil {
			return nil, err
		}
	}
	if err := client.SetImageFromBytes(buf.Bytes()); err != nil {
		return nil, err
	}

	text, err := client.Text()
	if err != nil {
		return nil, err
	}
	res := &ocr.Result{Text: text}
	if boxes, err := client.GetBoundingBoxes(gosseract.RIL_WORD); err == nil {
		res.Words = make([]ocr.Word, 0, len(boxes))
		for _, b := range boxes {
			res.Words = append(res.Words, ocr.Word{
				Text:       b.Word,
				Confidence: b.Confidence,
				Box:        b.Box,
			})
		}
	}
	return res, nil
}

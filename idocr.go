// Package idocr is a Go library for OCR-based extraction of Indonesian identity
// documents: KTP (national ID), NPWP (tax ID) and passports.
//
// The library separates two concerns:
//
//   - Text recognition is supplied by a pluggable [ocr.Engine]. The core has no
//     CGo dependency, so callers choose a backend (e.g. the optional
//     ocr/tesseract adapter, the built-in ocr/googlevision client, or a stub).
//   - Parsing turns recognised text into typed, validated structs. The
//     per-document Parse functions ([ktp.Parse], [npwp.Parse],
//     [passport.Parse]) are pure and can be tested without any OCR engine.
//
// A [Client] wires engines to the parsers. It can use a single engine for every
// document, or route each document type to a different engine. [NewAuto] applies
// the recommended routing — Google Vision for KTP and passport, Tesseract for
// NPWP — so callers need not pick an engine per call.
package idocr

import (
	"context"
	"fmt"
	"image"
	"os"

	"github.com/reynaldio/id-ocr/ktp"
	"github.com/reynaldio/id-ocr/npwp"
	"github.com/reynaldio/id-ocr/ocr"
	"github.com/reynaldio/id-ocr/ocr/googlevision"
	"github.com/reynaldio/id-ocr/passport"
)

// DocumentType enumerates the document kinds this library understands.
type DocumentType string

const (
	DocKTP      DocumentType = "ktp"
	DocNPWP     DocumentType = "npwp"
	DocPassport DocumentType = "passport"
)

// Client recognises and parses identity documents, routing each document type
// to a configured OCR engine.
type Client struct {
	defaultEngine ocr.Engine
	byType        map[DocumentType]ocr.Engine
}

// Option configures a Client.
type Option func(*Client)

// WithEngineFor routes a specific document type to a specific engine, overriding
// the default engine for that type.
func WithEngineFor(t DocumentType, engine ocr.Engine) Option {
	return func(c *Client) {
		if engine != nil {
			c.byType[t] = engine
		}
	}
}

// New returns a Client that uses engine for every document type, unless an
// Option overrides a specific type.
func New(engine ocr.Engine, opts ...Option) *Client {
	c := &Client{defaultEngine: engine, byType: make(map[DocumentType]ocr.Engine)}
	for _, o := range opts {
		o(c)
	}
	return c
}

// NewAuto returns a Client with the recommended per-document routing (verified
// on real cards):
//
//   - KTP and passport use cloud (Google Vision) when it is non-nil, otherwise
//     they fall back to local.
//   - NPWP uses local (Tesseract) when it is non-nil, otherwise cloud.
//
// Pair it with [VisionFromEnv] so KTP/passport use Vision only when an API key
// is configured:
//
//	client := idocr.NewAuto(idocr.VisionFromEnv(), tesseractEngine)
func NewAuto(cloud, local ocr.Engine) *Client {
	c := &Client{byType: make(map[DocumentType]ocr.Engine)}
	c.defaultEngine = firstNonNil(cloud, local)
	if cloud != nil {
		c.byType[DocKTP] = cloud
		c.byType[DocPassport] = cloud
	}
	if local != nil {
		c.byType[DocNPWP] = local
	}
	return c
}

// VisionFromEnv returns a Google Vision engine configured from the
// GOOGLE_VISION_API_KEY environment variable (with Indonesian + English hints),
// or nil when that variable is unset. It is a convenience for NewAuto.
func VisionFromEnv() ocr.Engine {
	key := os.Getenv("GOOGLE_VISION_API_KEY")
	if key == "" {
		return nil
	}
	return googlevision.New(
		googlevision.WithAPIKey(key),
		googlevision.WithLanguageHints("id", "en"),
	)
}

func firstNonNil(engines ...ocr.Engine) ocr.Engine {
	for _, e := range engines {
		if e != nil {
			return e
		}
	}
	return nil
}

func (c *Client) engineFor(t DocumentType) (ocr.Engine, error) {
	if e := c.byType[t]; e != nil {
		return e, nil
	}
	if c.defaultEngine != nil {
		return c.defaultEngine, nil
	}
	return nil, fmt.Errorf("idocr: no engine configured for document type %q", t)
}

// RecognizeKTP runs OCR on img (using the KTP engine) and parses it as a KTP.
func (c *Client) RecognizeKTP(ctx context.Context, img image.Image) (*ktp.KTP, error) {
	return run(c, ctx, img, DocKTP, ktp.Parse)
}

// RecognizeNPWP runs OCR on img (using the NPWP engine) and parses it as an NPWP.
func (c *Client) RecognizeNPWP(ctx context.Context, img image.Image) (*npwp.NPWP, error) {
	return run(c, ctx, img, DocNPWP, npwp.Parse)
}

// RecognizePassport runs OCR on img (using the passport engine) and parses it.
func (c *Client) RecognizePassport(ctx context.Context, img image.Image) (*passport.Passport, error) {
	return run(c, ctx, img, DocPassport, passport.Parse)
}

func run[T any](c *Client, ctx context.Context, img image.Image, t DocumentType, parse func(*ocr.Result) (T, error)) (T, error) {
	var zero T
	eng, err := c.engineFor(t)
	if err != nil {
		return zero, err
	}
	res, err := eng.Recognize(ctx, img)
	if err != nil {
		return zero, err
	}
	return parse(res)
}

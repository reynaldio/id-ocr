// Command example demonstrates id-ocr end to end with automatic engine routing.
//
// Usage:
//
//	example <ktp|npwp|passport> <image-file>
//
// KTP and passport use Google Vision when GOOGLE_VISION_API_KEY is set;
// otherwise (and always for NPWP) Tesseract is used. Tesseract needs the native
// library installed — see this folder's README.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG/PNG decoders for image.Decode
	_ "image/png"
	"os"

	idocr "github.com/reynaldio/id-ocr"
	"github.com/reynaldio/id-ocr/ocr/tesseract"
)

// document is any parsed result; every document type implements Validate.
type document interface{ Validate() error }

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: example <ktp|npwp|passport> <image-file>")
		os.Exit(2)
	}
	docType, path := os.Args[1], os.Args[2]

	img, err := loadImage(path)
	if err != nil {
		exit("load image: %v", err)
	}

	// One client, reused for every request. Auto routing picks the engine:
	// KTP/passport -> Google Vision (if an API key is configured), NPWP ->
	// Tesseract; with no API key everything falls back to Tesseract.
	client := idocr.NewAuto(idocr.VisionFromEnv(), tesseract.New())

	doc, err := recognize(client, context.Background(), docType, img)
	if err != nil {
		exit("recognize: %v", err)
	}

	out, _ := json.MarshalIndent(doc, "", "  ")
	fmt.Println(string(out))
	if err := doc.Validate(); err != nil {
		fmt.Println("\nvalidation: FAILED —", err)
	} else {
		fmt.Println("\nvalidation: OK")
	}
}

func recognize(c *idocr.Client, ctx context.Context, docType string, img image.Image) (document, error) {
	switch docType {
	case "ktp":
		return c.RecognizeKTP(ctx, img)
	case "npwp":
		return c.RecognizeNPWP(ctx, img)
	case "passport":
		return c.RecognizePassport(ctx, img)
	default:
		return nil, fmt.Errorf("unknown document type %q (want ktp|npwp|passport)", docType)
	}
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	// Sideways scan? Correct it before OCR, e.g. with
	//   img = preprocess.Rotate90(img, -1) // 90° left
	// (import github.com/reynaldio/id-ocr/preprocess).
	return img, nil
}

func exit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "example: "+format+"\n", args...)
	os.Exit(1)
}

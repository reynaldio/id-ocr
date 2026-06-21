// Command idtest runs the Tesseract-backed OCR pipeline on a local image file
// and prints the parsed, validated document. It's a manual test harness, not
// part of the library API.
//
// Usage:
//
//	go run ./cmd/idtest -type ktp   path/to/ktp.jpg
//	go run ./cmd/idtest -type npwp  path/to/npwp.jpg
//	go run ./cmd/idtest -type passport -raw path/to/passport.jpg
//
// -raw also dumps the unparsed OCR text, which is handy when a field doesn't
// come through and you want to see what Tesseract actually read.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"strings"
	"time"

	idocr "github.com/reynaldio/id-ocr"
	"github.com/reynaldio/id-ocr/ocr"
	"github.com/reynaldio/id-ocr/ocr/googlevision"
	"github.com/reynaldio/id-ocr/ocr/tesseract"
	"github.com/reynaldio/id-ocr/preprocess"
)

func main() {
	docType := flag.String("type", "ktp", "document type: ktp | npwp | passport")
	langs := flag.String("lang", "ind+eng", "tesseract languages, '+'-separated")
	showRaw := flag.Bool("raw", false, "also print the raw OCR text")
	psm := flag.Int("psm", 6, "tesseract page-segmentation mode (-1 = default)")
	scale := flag.Float64("scale", 2.5, "upscale factor applied to a grayscaled image (1 = none)")
	dump := flag.String("dump", "", "write the preprocessed image to this PNG path and exit")
	asJSON := flag.Bool("json", false, "print the parsed struct as indented JSON")
	engineName := flag.String("engine", "auto", "ocr engine: auto | tesseract | googlevision")
	apiKey := flag.String("apikey", os.Getenv("GOOGLE_VISION_API_KEY"), "Google Vision API key (or $GOOGLE_VISION_API_KEY)")
	rotate := flag.Int("rotate", 0, "rotate the image clockwise by this many degrees (90/180/270; negative = left) before OCR")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: idtest -type <ktp|npwp|passport> [-raw] <image>")
		os.Exit(2)
	}
	path := flag.Arg(0)

	img, err := loadImage(path)
	if err != nil {
		fatal("load image: %v", err)
	}
	if *rotate != 0 {
		img = preprocess.Rotate90(img, *rotate/90)
	}

	if *dump != "" {
		dumpPreprocessed(img, *scale, *dump)
		return
	}

	engine, err := buildEngine(*engineName, *docType, *langs, *psm, *scale, *apiKey)
	if err != nil {
		fatal("%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if *showRaw {
		if res, err := engine.Recognize(ctx, img); err == nil {
			fmt.Printf("===== RAW OCR TEXT =====\n%s\n========================\n\n", res.Text)
		}
	}

	doc, err := recognize(idocr.New(engine), ctx, *docType, img)
	if err != nil {
		fatal("recognize: %v", err)
	}

	if *asJSON {
		b, _ := json.MarshalIndent(doc, "", "  ")
		fmt.Printf("%s\n", b)
	} else {
		fmt.Printf("===== PARSED %s =====\n%+v\n", *docType, doc)
	}
	if err := doc.Validate(); err != nil {
		fmt.Printf("\nVALIDATION: FAILED — %v\n", err)
	} else {
		fmt.Printf("\nVALIDATION: OK\n")
	}
}

func dumpPreprocessed(img image.Image, scale float64, path string) {
	pre := preprocess.Document(img, scale)
	f, err := os.Create(path)
	if err != nil {
		fatal("dump: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, pre); err != nil {
		fatal("dump encode: %v", err)
	}
	b := pre.Bounds()
	fmt.Printf("wrote %s (%dx%d)\n", path, b.Dx(), b.Dy())
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// buildEngine constructs the selected OCR engine.
func buildEngine(name, docType, langs string, psm int, scale float64, apiKey string) (ocr.Engine, error) {
	tess := func() ocr.Engine {
		return tesseract.New(
			tesseract.WithLanguages(strings.Split(langs, "+")...),
			tesseract.WithPageSegMode(psm),
			tesseract.WithPreprocessor(func(im image.Image) image.Image {
				return preprocess.Document(im, scale)
			}),
		)
	}
	vision := func() (ocr.Engine, error) {
		if apiKey == "" {
			return nil, fmt.Errorf("googlevision needs -apikey or $GOOGLE_VISION_API_KEY")
		}
		return googlevision.New(
			googlevision.WithAPIKey(apiKey),
			googlevision.WithLanguageHints("id", "en"),
		), nil
	}

	switch name {
	case "tesseract":
		return tess(), nil
	case "googlevision":
		return vision()
	case "auto":
		// Recommended routing: NPWP -> Tesseract; KTP/passport -> Vision when an
		// API key exists, otherwise Tesseract.
		if docType != "npwp" && apiKey != "" {
			return vision()
		}
		return tess(), nil
	default:
		return nil, fmt.Errorf("unknown -engine %q (want auto|tesseract|googlevision)", name)
	}
}

// recognized is any parsed document that can validate itself.
type recognized interface{ Validate() error }

func recognize(c *idocr.Client, ctx context.Context, docType string, img image.Image) (recognized, error) {
	switch docType {
	case "ktp":
		return c.RecognizeKTP(ctx, img)
	case "npwp":
		return c.RecognizeNPWP(ctx, img)
	case "passport":
		return c.RecognizePassport(ctx, img)
	default:
		return nil, fmt.Errorf("unknown -type %q (want ktp|npwp|passport)", docType)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "idtest: "+format+"\n", args...)
	os.Exit(1)
}

# id-ocr

A Go library for OCR-based extraction of **Indonesian identity documents** into
typed, validated structs. Designed to be embedded in other services.

Supported documents:

| Document   | Package                         | Highlights                                        |
| ---------- | ------------------------------- | ------------------------------------------------- |
| **KTP**    | `ktp`                           | NIK structure + embedded birth-date validation    |
| **NPWP**   | `npwp`                          | 15-digit number formatting, optional NIK (2024+)  |
| **Passport** | `passport`                    | ICAO TD3 MRZ decode with full check-digit checks  |

More document types can be added later behind the same pattern.

## Design

The library separates **text recognition** from **parsing**:

- **OCR is pluggable.** The core depends only on the small [`ocr.Engine`](ocr/ocr.go)
  interface, so it has **no CGo or cloud dependencies**. Bring your own backend:
  the optional [Tesseract adapter](ocr/tesseract/), a cloud Vision client, or a
  test stub.
- **Parsers are pure functions** over recognised text
  ([`ktp.Parse`](ktp/parser.go), [`npwp.Parse`](npwp/npwp.go),
  [`passport.Parse`](passport/mrz.go)). They need no OCR engine and are fully
  unit-tested.

```text
image ──▶ ocr.Engine ──▶ ocr.Result(text) ──▶ <doc>.Parse ──▶ typed struct ──▶ .Validate()
```

## Install

```sh
go get github.com/reynaldio/id-ocr@v0.2.0
```

## Usage

### Automatic engine routing (recommended)

`NewAuto` routes each document to the engine that handles it best, so you don't
pick an engine per call: **KTP and passport → Google Vision** (when an API key is
configured), **NPWP → Tesseract**. If no Vision API key is present, everything
falls back to Tesseract.

```go
import (
    "context"

    idocr "github.com/reynaldio/id-ocr"
    "github.com/reynaldio/id-ocr/ocr/tesseract"
)

// tesseract.New() needs no configuration — it defaults to Indonesian+English,
// PSM 6, and grayscale+upscale preprocessing. VisionFromEnv() reads
// GOOGLE_VISION_API_KEY (nil if unset, so KTP/passport then use Tesseract too).
client := idocr.NewAuto(idocr.VisionFromEnv(), tesseract.New())

ktp, err := client.RecognizeKTP(context.Background(), img) // img is image.Image
if err != nil { /* ... */ }
if err := ktp.Validate(); err != nil { /* low-confidence / inconsistent scan */ }
fmt.Println(ktp.NIK, ktp.Name, ktp.BirthDate)
```

### A single engine for everything

```go
client := idocr.New(tesseract.New()) // sensible defaults baked in
// ...or route specific types yourself:
client = idocr.New(visionEngine, idocr.WithEngineFor(idocr.DocNPWP, tesseract.New()))
```

### Runnable example

A complete program lives in [`examples/`](examples/) (its own module, since it
links the CGo Tesseract adapter):

```sh
cd examples
export GOOGLE_VISION_API_KEY='...'   # KTP/passport via Vision; omit for Tesseract-only
go run . ktp      path/to/ktp.jpg
go run . npwp     path/to/npwp.jpg
go run . passport path/to/passport.jpg
```

It prints the parsed struct as JSON and the validation result. See
[examples/README.md](examples/README.md) for the Vision-only (no CGo) variant.

### Parsing text you already have

If OCR happens elsewhere, parse the text directly — no engine required:

```go
res := &ocr.Result{Text: rawOCRText}
p, _ := passport.Parse(res)
fmt.Println(p.Number, p.Surname, p.GivenNames, p.Checks.Valid())
```

### Implementing your own engine

```go
type myVision struct{ /* ... */ }

func (m myVision) Recognize(ctx context.Context, img image.Image) (*ocr.Result, error) {
    text := callCloudVision(ctx, img)
    return &ocr.Result{Text: text}, nil
}

client := idocr.New(myVision{})
```

## Image preprocessing

Photographed cards OCR poorly at native resolution. The dependency-light
[`preprocess`](preprocess/preprocess.go) package (only `golang.org/x/image`)
provides grayscale conversion, high-quality Catmull-Rom upscaling, Otsu
binarization, and sharpening. `preprocess.Document(img, 2.5)` (grayscale +
2.5× upscale) is a good default and is wired into the Tesseract engine via
`WithPreprocessor`. Upscaling small fonts is the single biggest accuracy win.

## The Tesseract adapter

`ocr/tesseract` is a **separate nested module** so its CGo + native Tesseract
dependency never touches the core module.

```sh
# system dependency
brew install tesseract tesseract-lang  # macOS (tesseract-lang includes 'ind')
# or: apt-get install tesseract-ocr libtesseract-dev tesseract-ocr-ind

go get github.com/reynaldio/id-ocr/ocr/tesseract
```

The engine is configurable:

```go
engine := tesseract.New(
    tesseract.WithLanguages("ind", "eng"),
    tesseract.WithPageSegMode(6), // single uniform block — good for cards
    tesseract.WithPreprocessor(func(im image.Image) image.Image {
        return preprocess.Document(im, 2.5)
    }),
)
```

**Building the CGo adapter on macOS / Apple Silicon.** Homebrew installs under
`/opt/homebrew`, which gosseract's CGo flags don't include by default, so set:

```sh
export CGO_CPPFLAGS="-I$(brew --prefix tesseract)/include -I$(brew --prefix leptonica)/include"
export CGO_LDFLAGS="-L$(brew --prefix tesseract)/lib -L$(brew --prefix leptonica)/lib"
go build ./...
```

### Manual test harness

`ocr/tesseract/cmd/idtest` runs the full pipeline on a local image of your own:

```sh
cd ocr/tesseract
go run ./cmd/idtest -type ktp  -json  path/to/ktp.jpg
go run ./cmd/idtest -type npwp -raw   path/to/npwp.jpg   # also print raw OCR text
go run ./cmd/idtest -dump /tmp/pre.png path/to/ktp.jpg   # inspect preprocessing
```

## OCR accuracy & engine choice

Accuracy is bounded by the OCR engine, not the parsers, and the best engine
differs **per document**:

| Document | Recommended engine | Why |
| --- | --- | --- |
| **KTP** | **Google Vision** | small fonts over a colored security pattern; Vision reads the NIK and fields accurately where local Tesseract misreads digits |
| **NPWP** | **Tesseract** | large high-contrast digits; Tesseract's grayscale+upscale washes out the heavy "DIREKTORAT JENDERAL PAJAK" watermark that pollutes Vision's output |
| **Passport** | **Google Vision** | the TD3 MRZ needs the "<" fillers preserved, which Vision reads and Tesseract loses |

Because the engine is just an [`ocr.Engine`](ocr/ocr.go), you can pick one per
[`DocumentType`](idocr.go) — e.g. Vision for KTP/passport, Tesseract for NPWP —
or supply your own (AWS Textract, a trained model, ...). The parsers,
validation, and preprocessing are unchanged; only the engine swaps.

`Validate()` is the safety net: when an engine misreads a check-protected field
(NIK structure, MRZ check digits) it is rejected rather than passing through.

## The Google Vision adapter

[`ocr/googlevision`](ocr/googlevision/googlevision.go) calls the Google Cloud
Vision REST API (`images:annotate`, `DOCUMENT_TEXT_DETECTION`). It uses **only
the standard library** — no CGo, no third-party SDK — so it lives in the core
module. It is the recommended engine for KTP and passport accuracy.

```go
engine := googlevision.New(
    googlevision.WithAPIKey(os.Getenv("GOOGLE_VISION_API_KEY")),
    googlevision.WithLanguageHints("id", "en"),
)
ktp, err := idocr.New(engine).RecognizeKTP(ctx, img)
```

Authenticate with `WithAPIKey` (a Vision API key) or `WithBearerToken` (an
OAuth2 / service-account access token, e.g.
`gcloud auth application-default print-access-token`). Vision is a paid Google
Cloud API; see [its pricing](https://cloud.google.com/vision/pricing).

## Output & field normalization

Parsed structs use normalized, typed values rather than raw OCR text:

- **Sex** is the shared [`idtype.Sex`](idtype/idtype.go) type with values
  `MALE` / `FEMALE` (and `UNSPECIFIED` for a passport "X"), identical across KTP
  and passport. On a KTP the printed *Jenis Kelamin* field is the primary
  source; the NIK is only a fallback when that field is unreadable.
- **Dates** are `time.Time` in Go and render **date-only** (`"1981-06-26"`) in
  JSON — KTP `BirthDate`; passport `BirthDate` / `ExpiryDate`.
- **KTP validity** is split: `ValidUntilText` (as printed, e.g. `"SEUMUR HIDUP"`
  or a date) and `ValidUntilDate` (`time.Time`). A lifetime card maps
  `ValidUntilDate` to `ktp.LifetimeDate` = `9999-12-31` (the max calendar date),
  so "not expired" comparisons just work.
- **Passport** fields come from two zones: the **MRZ** (check-digit verified —
  number, nationality, DOB, sex, expiry, plus `PersonalNumber` from the MRZ
  optional-data field) and the printed **visual zone** (`RegistrationNumber`
  / "No. Reg" and `IssuingOffice`), which are **not** check-protected.
- `RawText` is excluded from JSON (`json:"-"`).

## Validation

Every document exposes `Validate() error`:

- **KTP** — NIK length/digits, region & sequence sanity, valid embedded birth
  date, and that the NIK-encoded date matches the printed birth date.
- **NPWP** — 15-digit number; 16-digit NIK when present.
- **Passport** — every ICAO 9303 MRZ check digit (number, DOB, expiry, personal
  number, composite) via `Passport.Checks`.

## Versioning

Follows [Semantic Versioning](https://semver.org). The current version is
**v0.2.0**, exposed as [`idocr.Version`](version.go) and mirroring the published
git tag. The Tesseract submodule is tagged in parallel
(`ocr/tesseract/v0.2.0`). While on `v0.x` the public API may change between
minor releases.

## Development

```sh
go test ./...     # core: pure Go, only golang.org/x/image, no CGo/native deps
gofmt -l .        # formatting
go vet ./...
```

The `ocr/tesseract` adapter is a separate module with its own `go.mod`; build
it with the CGo flags shown above.

## License

MIT

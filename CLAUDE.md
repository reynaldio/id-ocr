# id-ocr — notes for Claude

Go library extracting Indonesian ID documents (KTP, NPWP, passport) into typed,
validated structs. Consumed by other projects as a dependency.

## Architecture

- **OCR is pluggable, parsing is pure.** Core depends only on the `ocr.Engine`
  interface — no CGo, no cloud SDKs. Each `<doc>.Parse(*ocr.Result)` is a pure
  function over recognised text and is unit-tested without any engine.
- `idocr.Client` (root package) wires an `ocr.Engine` to the parsers for
  image → struct convenience.
- Document packages: `ktp`, `npwp`, `passport`. Shared helpers in
  `internal/normalize` (whitespace, Indonesian dates, label stripping).
- `idtype` (core) holds value types shared across documents — currently
  `Sex` (`MALE`/`FEMALE`/`UNSPECIFIED`). Lives in its own package because
  `ktp`/`passport` can't import the root (cycle).
- `ocr/tesseract` is a **nested module** (own `go.mod`) so its gosseract/CGo
  dependency never enters the core module. It has PSM (`WithPageSegMode`) and
  preprocessing (`WithPreprocessor`) options. `cmd/idtest` is a manual harness.
- `ocr/googlevision` (core, stdlib-only) is the Vision REST adapter — no CGo, no
  SDK; API key or bearer token. Recommended engine for KTP/passport accuracy.
- `preprocess` (core) cleans images before OCR: grayscale, Catmull-Rom upscale,
  Otsu binarize, sharpen. Only dep is `golang.org/x/image`. `Document(img, 2.5)`
  is the default (grayscale + upscale only — sharpen/binarize amplify the KTP
  security-pattern noise, so they're opt-in).

## Conventions

- Parsers are **best-effort**: missing/unreadable fields stay zero-valued, never
  error. Correctness is checked separately via `Validate()`.
- Every document type exposes `Validate() error`.
- **Normalized output**: `Sex` is `idtype.Sex` (MALE/FEMALE). Dates are
  `time.Time` but marshal **date-only** via per-struct `MarshalJSON` (KTP
  `BirthDate`; passport `BirthDate`/`ExpiryDate`). KTP validity is split into
  `ValidUntilText` + `ValidUntilDate`; "SEUMUR HIDUP" → `ktp.LifetimeDate`
  (9999-12-31). Passport `RegistrationNumber`/`IssuingOffice` come from the
  printed visual zone (not the MRZ, so not check-protected); `PersonalNumber`
  is the MRZ optional-data field. `RawText` is `json:"-"`.
- Keep the core module dependency-free (stdlib only). Heavy/native deps go in
  nested modules under their own `go.mod`.
- Adding a document type: new package + `Parse(*ocr.Result)` + typed struct +
  `Validate()` + a `Recognize*` method on `idocr.Client` + tests with sample
  OCR text.

## Domain facts worth keeping straight

- **NIK** (16 digits): `PPKKCC DDMMYY SSSS`. Day-of-month is offset by **+40 for
  women**. Two-digit year is pivoted around the current year.
- **NPWP**: 15 digits, `XX.XXX.XXX.X-XXX.XXX`. Post-2024 cards may also carry a
  16-digit NIK — detect the NIK *before* the NPWP number when scanning.
- **Passport MRZ** is ICAO **TD3**: two 44-char lines. Check digits use the
  repeating **7-3-1** weighting (`A=10..Z=35`, filler `<`=0). The MRZ is the
  primary, check-digit-verified source. `passport/visual.go` is a **fallback**:
  it fills fields the MRZ lacks (issue date, birth place) and recovers core
  fields from the printed visual zone when the MRZ is unreadable — those are
  best-effort and unverified, so `Checks` stays all-false and `Validate()` fails.
  `findMRZ` also reassembles MRZ rows split into fragments (validated by check
  digits). Sideways scans: `preprocess.Rotate90` / `idtest -rotate`.

## Commands

```sh
go test ./...        # core
gofmt -l . && go vet ./...
# Tesseract adapter (separate module, needs native libtesseract).
# macOS/Apple Silicon: Homebrew is under /opt/homebrew, not in gosseract's
# default CGo paths, so set the flags:
export CGO_CPPFLAGS="-I$(brew --prefix tesseract)/include -I$(brew --prefix leptonica)/include"
export CGO_LDFLAGS="-L$(brew --prefix tesseract)/lib -L$(brew --prefix leptonica)/lib"
cd ocr/tesseract && go mod tidy && go build ./...
```

## OCR accuracy & engine-per-document (verified on real cards, 2026-06-21)

Engine-bound, not parser-bound. The best engine differs per document — pick one
per `DocumentType`:

- **KTP → Google Vision.** Vision reads the NIK and fields accurately; local
  Tesseract misreads digits (small fonts over a colored security pattern), even
  with tuned preprocessing.
- **NPWP → Tesseract.** Its grayscale+upscale washes out the heavy "DIREKTORAT
  JENDERAL PAJAK" watermark that pollutes Vision's Name/Address; the number/NIK
  are large and read cleanly.
- **Passport → Google Vision.** The TD3 MRZ needs the "<" fillers, which Vision
  preserves and Tesseract loses.

Vision quirks the code already handles: it serialises form layouts with columns
interleaved (fixed by `ocr.ReconstructText` + deskew) and tokenises the MRZ with
spaces / drops trailing fillers (fixed by `normalizeMRZ` dropping spaces and
`mrzLine` padding to 44). `Validate()` rejects check-protected misreads (NIK
structure, MRZ check digits).

## Engine routing

`idocr.Client` routes per `DocumentType`. `New(engine, opts...)` uses one engine
(override per type with `WithEngineFor`); `NewAuto(cloud, local)` applies the
recommended routing (KTP/passport→cloud Vision, NPWP→local Tesseract; cloud nil
⇒ all Tesseract). `VisionFromEnv()` builds a Vision engine from
`GOOGLE_VISION_API_KEY` (idocr may import the stdlib-only googlevision adapter).
`tesseract.New()` ships working defaults (ind+eng, PSM 6, Document preprocess).

## Layout

Core module is CGo-free. Nested modules (own `go.mod`): `ocr/tesseract` (CGo
adapter + `cmd/idtest`) and `examples` (runnable demo). Both `replace` core to
local for dev.

## Versioning

SemVer. Current: **v0.2.0**. Bump `Version` in `version.go`, the Tesseract
submodule's core `require`, and tag both `vX.Y.Z` and `ocr/tesseract/vX.Y.Z`
together. On `v0.x` the API may break between minor versions.

# id-ocr example

A runnable end-to-end example using automatic engine routing.

```sh
# KTP and passport use Google Vision when this is set; otherwise Tesseract.
export GOOGLE_VISION_API_KEY='...'

# Tesseract is CGo + native libs. Install them, then set the build flags:
#   macOS:  brew install tesseract tesseract-lang
#   Debian: apt-get install tesseract-ocr libtesseract-dev tesseract-ocr-ind
export CGO_CPPFLAGS="-I$(brew --prefix tesseract)/include -I$(brew --prefix leptonica)/include"
export CGO_LDFLAGS="-L$(brew --prefix tesseract)/lib -L$(brew --prefix leptonica)/lib"

go run . ktp      path/to/ktp.jpg
go run . npwp     path/to/npwp.jpg
go run . passport path/to/passport.jpg
```

It prints the parsed struct as JSON and whether validation passed.

This is a **separate Go module** (its own `go.mod`) because it imports the CGo
Tesseract adapter; the `replace` directives point at the local source. In your
own project you would instead:

```sh
go get github.com/reynaldio/id-ocr
go get github.com/reynaldio/id-ocr/ocr/tesseract   # only if you want Tesseract
```

## Vision-only (no CGo, no Tesseract install)

If you only use Google Vision, drop the Tesseract import and build flags:

```go
client := idocr.New(idocr.VisionFromEnv())
```

Vision handles all three document types (KTP/passport best; NPWP's number/NIK
read fine, its name/address pick up watermark noise).

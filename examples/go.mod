// Standalone example module. It imports both the core library and the CGo
// Tesseract adapter, so it is kept separate from the core module's build.
// The replace directives point at the local source for development.
module github.com/reynaldio/id-ocr/examples

go 1.26.3

require (
	github.com/reynaldio/id-ocr v0.1.0
	github.com/reynaldio/id-ocr/ocr/tesseract v0.0.0-00010101000000-000000000000
)

require (
	github.com/otiai10/gosseract/v2 v2.4.1 // indirect
	golang.org/x/image v0.43.0 // indirect
)

replace (
	github.com/reynaldio/id-ocr => ../
	github.com/reynaldio/id-ocr/ocr/tesseract => ../ocr/tesseract
)

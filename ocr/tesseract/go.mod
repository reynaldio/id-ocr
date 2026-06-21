// Nested module: the optional Tesseract (CGo) adapter. Kept separate so the
// core github.com/reynaldio/id-ocr module has zero CGo/native dependencies.
//
// Run `go mod tidy` here to pin gosseract once you have network access and a
// system Tesseract install. The replace directive points at the local core
// module so the two build together during development.
module github.com/reynaldio/id-ocr/ocr/tesseract

go 1.26.3

require (
	github.com/otiai10/gosseract/v2 v2.4.1
	github.com/reynaldio/id-ocr v0.2.0
)

require golang.org/x/image v0.43.0 // indirect

replace github.com/reynaldio/id-ocr => ../../

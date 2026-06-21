// Package preprocess provides dependency-free image cleanup that materially
// improves OCR accuracy on photographed ID cards: grayscale conversion,
// bilinear upscaling (small fonts OCR far better when enlarged), and optional
// Otsu binarization.
//
// It operates on the standard library image types only, so it is safe to use
// from the core module and from any OCR engine adapter.
package preprocess

import (
	"image"
	"image/color"

	"golang.org/x/image/draw"
)

// Rotate90 rotates img by the given number of 90° quarter-turns clockwise
// (negative turns are counter-clockwise; e.g. -1 rotates 90° to the left). It
// is lossless, for correcting a sideways scan before OCR.
func Rotate90(img image.Image, turns int) image.Image {
	turns = ((turns % 4) + 4) % 4
	if turns == 0 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := h, w // 90°/270° swap the dimensions
	if turns == 2 {
		dw, dh = w, h
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := rotatedPoint(turns, x, y, w, h)
			dst.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// rotatedPoint maps a source pixel to its destination for a clockwise rotation
// of `turns` quarter-turns.
func rotatedPoint(turns, x, y, w, h int) (int, int) {
	switch turns {
	case 1: // 90° clockwise
		return h - 1 - y, x
	case 2: // 180°
		return w - 1 - x, h - 1 - y
	default: // 270° clockwise == 90° counter-clockwise
		return y, w - 1 - x
	}
}

// Grayscale converts img to an 8-bit grayscale image using Rec. 601 luma
// (the conversion built into color.GrayModel).
func Grayscale(img image.Image) *image.Gray {
	b := img.Bounds()
	g := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			g.Set(x, y, img.At(x, y))
		}
	}
	return g
}

// ScaleGray resizes src by factor using high-quality Catmull-Rom resampling.
// A factor > 1 enlarges (small OCR fonts read far better enlarged); values
// <= 0 or == 1 return src unchanged. The sharper cubic kernel preserves the
// thin strokes of the NIK/MRZ fonts that a plain bilinear filter blurs away.
func ScaleGray(src *image.Gray, factor float64) *image.Gray {
	if factor <= 0 || factor == 1 {
		return src
	}
	sb := src.Bounds()
	dw := int(float64(sb.Dx()) * factor)
	dh := int(float64(sb.Dy()) * factor)
	if dw < 1 || dh < 1 {
		return src
	}
	dst := image.NewGray(image.Rect(0, 0, dw, dh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, sb, draw.Over, nil)
	return dst
}

// OtsuThreshold computes the optimal global threshold for src using Otsu's
// method (maximising between-class variance of the intensity histogram).
func OtsuThreshold(src *image.Gray) uint8 {
	var hist [256]int
	b := src.Bounds()
	total := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			hist[src.GrayAt(x, y).Y]++
			total++
		}
	}
	if total == 0 {
		return 128
	}
	var sum float64
	for i, c := range hist {
		sum += float64(i * c)
	}
	var sumB float64
	var wB int
	var maxVar float64
	thr := 0
	for i := 0; i < 256; i++ {
		wB += hist[i]
		if wB == 0 {
			continue
		}
		wF := total - wB
		if wF == 0 {
			break
		}
		sumB += float64(i * hist[i])
		mB := sumB / float64(wB)
		mF := (sum - sumB) / float64(wF)
		v := float64(wB) * float64(wF) * (mB - mF) * (mB - mF)
		if v > maxVar {
			maxVar = v
			thr = i
		}
	}
	return uint8(thr)
}

// Binarize maps pixels above thr to white and the rest to black.
func Binarize(src *image.Gray, thr uint8) *image.Gray {
	b := src.Bounds()
	dst := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if src.GrayAt(x, y).Y > thr {
				dst.SetGray(x, y, color.Gray{Y: 255})
			} else {
				dst.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}
	return dst
}

// Sharpen applies a 3x3 unsharp convolution that crispens edges. Upscaling
// softens the thin strokes of the NIK and MRZ fonts; sharpening afterwards
// restores the contrast Tesseract needs to read them.
func Sharpen(src *image.Gray) *image.Gray {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewGray(b)
	at := func(x, y int) int {
		if x < 0 {
			x = 0
		} else if x >= w {
			x = w - 1
		}
		if y < 0 {
			y = 0
		} else if y >= h {
			y = h - 1
		}
		return int(src.GrayAt(b.Min.X+x, b.Min.Y+y).Y)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := 5*at(x, y) - at(x, y-1) - at(x, y+1) - at(x-1, y) - at(x+1, y)
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			dst.SetGray(b.Min.X+x, b.Min.Y+y, color.Gray{Y: uint8(v)})
		}
	}
	return dst
}

// Document applies the default ID-card pipeline: grayscale, then high-quality
// upscale by scale. It deliberately stops short of Sharpen and Binarize: both
// help clean scans but amplify the security-pattern noise on a photographed
// card. Compose them explicitly when the input warrants it, e.g.
//
//	g := preprocess.Document(img, 2.5)
//	g = preprocess.Binarize(g, preprocess.OtsuThreshold(g))
func Document(img image.Image, scale float64) *image.Gray {
	return ScaleGray(Grayscale(img), scale)
}

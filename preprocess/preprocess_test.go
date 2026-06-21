package preprocess

import (
	"image"
	"image/color"
	"testing"
)

func solid(w, h int, y uint8) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = y
	}
	return g
}

func TestScaleGrayDimensions(t *testing.T) {
	src := solid(10, 20, 128)
	dst := ScaleGray(src, 2.5)
	if dst.Bounds().Dx() != 25 || dst.Bounds().Dy() != 50 {
		t.Fatalf("scaled size = %v, want 25x50", dst.Bounds())
	}
	// A solid image must stay solid after bilinear scaling.
	if got := dst.GrayAt(12, 30).Y; got != 128 {
		t.Errorf("center pixel = %d, want 128", got)
	}
}

func TestSharpenIncreasesEdgeContrast(t *testing.T) {
	// A single bright pixel on a mid-gray field should get brighter (its
	// center weight dominates) after sharpening.
	g := solid(5, 5, 100)
	g.SetGray(2, 2, color.Gray{Y: 200})
	out := Sharpen(g)
	if out.GrayAt(2, 2).Y <= 200 {
		t.Errorf("sharpened center = %d, want > 200", out.GrayAt(2, 2).Y)
	}
}

func TestRotate90(t *testing.T) {
	// 3x2 image; mark the top-left pixel so we can track where it lands.
	src := image.NewGray(image.Rect(0, 0, 3, 2))
	for i := range src.Pix {
		src.Pix[i] = 100
	}
	src.SetGray(0, 0, color.Gray{Y: 255})

	// 90° clockwise: dims swap to 2x3, top-left goes to top-right.
	cw := Rotate90(src, 1)
	if cw.Bounds().Dx() != 2 || cw.Bounds().Dy() != 3 {
		t.Fatalf("90° size = %v, want 2x3", cw.Bounds())
	}
	if r, _, _, _ := cw.At(1, 0).RGBA(); r>>8 != 255 {
		t.Errorf("top-left pixel did not land top-right after 90° CW")
	}

	// Four 90° turns return to the original orientation and content.
	round := Rotate90(Rotate90(Rotate90(Rotate90(src, 1), 1), 1), 1)
	if r, _, _, _ := round.At(0, 0).RGBA(); r>>8 != 255 {
		t.Errorf("4x90° did not restore the marked pixel")
	}
	// -1 (CCW) equals 3 (CW).
	if Rotate90(src, -1).Bounds() != Rotate90(src, 3).Bounds() {
		t.Errorf("turns normalization wrong")
	}
}

func TestScaleGrayNoop(t *testing.T) {
	src := solid(4, 4, 50)
	if ScaleGray(src, 1) != src {
		t.Error("factor 1 should return the source unchanged")
	}
}

func TestOtsuAndBinarize(t *testing.T) {
	// Two intensity clusters at 60 and 200; Otsu should split between them.
	const dark, light = 60, 200
	g := image.NewGray(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			v := uint8(dark)
			if x >= 5 {
				v = light
			}
			g.SetGray(x, y, color.Gray{Y: v})
		}
	}
	thr := OtsuThreshold(g)
	if thr < dark || thr >= light {
		t.Fatalf("threshold = %d, want in [%d,%d)", thr, dark, light)
	}
	bin := Binarize(g, thr)
	if bin.GrayAt(0, 0).Y != 0 || bin.GrayAt(9, 0).Y != 255 {
		t.Errorf("binarize: left=%d right=%d", bin.GrayAt(0, 0).Y, bin.GrayAt(9, 0).Y)
	}
}

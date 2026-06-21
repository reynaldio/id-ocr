package ocr

import (
	"image"
	"math"
	"testing"
)

func word(text string, x, y int) Word {
	// 60x20 box anchored at (x, y).
	return Word{Text: text, Box: image.Rect(x, y, x+60, y+20)}
}

func TestReconstructText_RegroupsColumns(t *testing.T) {
	// Mimic Vision's interleaved order: all labels first, then all values,
	// even though geometrically each label/value pair shares a row.
	words := []Word{
		word("NIK", 10, 100),
		word("Nama", 10, 140),
		word("3201015708950002", 200, 100),
		word("SITI", 200, 140),
		word("AMINAH", 280, 140),
	}
	got := ReconstructText(words)
	want := "NIK 3201015708950002\nNama SITI AMINAH"
	if got != want {
		t.Errorf("ReconstructText =\n%q\nwant\n%q", got, want)
	}
}

// centeredWord builds a 50x20 word box centred at (cx, cy).
func centeredWord(text string, cx, cy int) Word {
	return Word{Text: text, Box: image.Rect(cx-25, cy-10, cx+25, cy+10)}
}

func TestReconstructText_Deskew(t *testing.T) {
	// Three label/value rows, then rotate every word centre by +9° to simulate
	// a card photographed at an angle. Within a row the value sits ~190px right
	// of the label, so the skew shifts it ~30px in Y — more than a row height —
	// which would merge/split rows without deskew.
	rows := [][2]string{
		{"NIK", "3201015708950002"},
		{"Nama", "SITI"},
		{"Alamat", "JALAN"},
	}
	const labelX, valueX = 40, 230
	a := 9.0 * math.Pi / 180
	sin, cos := math.Sincos(a)
	skew := func(x, y int) (int, int) {
		return int(float64(x)*cos - float64(y)*sin), int(float64(x)*sin + float64(y)*cos)
	}

	var words []Word
	for r, pair := range rows {
		y := 120 + r*70
		lx, ly := skew(labelX, y)
		vx, vy := skew(valueX, y)
		// Append value before label to also exercise the column regrouping.
		words = append(words, centeredWord(pair[1], vx, vy), centeredWord(pair[0], lx, ly))
	}

	got := ReconstructText(words)
	want := "NIK 3201015708950002\nNama SITI\nAlamat JALAN"
	if got != want {
		t.Errorf("ReconstructText (deskew) =\n%q\nwant\n%q", got, want)
	}
}

func TestReconstructText_NoGeometry(t *testing.T) {
	if got := ReconstructText([]Word{{Text: "hello"}}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

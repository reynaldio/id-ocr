package ocr

import (
	"math"
	"sort"
	"strings"
)

// positionedWord is an OCR word reduced to the geometry ReconstructText needs.
type positionedWord struct {
	text   string
	x, y   float64 // box centre, in image pixels
	height int
	rx, ry float64 // centre after deskew rotation
}

// ReconstructText rebuilds readable, row-ordered text from positioned words.
//
// Some engines (notably Google Vision on form layouts like a KTP) serialize
// their full-text output in an order that interleaves the label column and the
// value column, which breaks "Label : value" line parsing. The per-word
// bounding boxes, however, carry the true geometry. ReconstructText estimates
// and removes any skew (a card photographed at an angle), groups words into
// rows by vertical position, and orders each row left-to-right, so a label and
// its value land on the same line again.
//
// Words without a bounding box are ignored. If no word has geometry the result
// is empty and callers should fall back to the engine's own text.
func ReconstructText(words []Word) string {
	pw := positionedWords(words)
	if len(pw) == 0 {
		return ""
	}

	theta := estimateSkew(pw, medianHeight(pw))
	sin, cos := math.Sincos(theta)
	for i := range pw {
		// Rotate the centre by -theta to level the rows.
		pw[i].rx = pw[i].x*cos + pw[i].y*sin
		pw[i].ry = -pw[i].x*sin + pw[i].y*cos
	}

	return groupRows(pw, medianHeight(pw))
}

func positionedWords(words []Word) []positionedWord {
	out := make([]positionedWord, 0, len(words))
	for _, w := range words {
		b := w.Box
		if b.Empty() || strings.TrimSpace(w.Text) == "" {
			continue
		}
		out = append(out, positionedWord{
			text:   w.Text,
			x:      float64(b.Min.X+b.Max.X) / 2,
			y:      float64(b.Min.Y+b.Max.Y) / 2,
			height: b.Dy(),
		})
	}
	return out
}

func medianHeight(pw []positionedWord) int {
	hs := make([]int, len(pw))
	for i, p := range pw {
		hs[i] = p.height
	}
	sort.Ints(hs)
	h := hs[len(hs)/2]
	if h < 1 {
		h = 1
	}
	return h
}

// estimateSkew finds the rotation (radians, in [-15°, +15°]) that best aligns
// the words into horizontal rows, using a projection-profile score: rotate the
// word centres by a trial angle, bin their Y, and prefer the angle where the
// bins are peakiest (words pile onto a few shared rows). Returns 0 when there
// are too few words to estimate reliably.
func estimateSkew(pw []positionedWord, medianH int) float64 {
	if len(pw) < 6 {
		return 0
	}
	bin := float64(medianH) / 4
	if bin < 1 {
		bin = 1
	}
	best, bestScore := 0.0, -1.0
	for deg := -1500; deg <= 1500; deg += 25 { // -15.00°..15.00° in 0.25° steps
		theta := float64(deg) / 100 * math.Pi / 180
		sin, cos := math.Sincos(theta)
		counts := make(map[int]int, len(pw))
		for _, p := range pw {
			yp := -p.x*sin + p.y*cos
			counts[int(yp/bin)]++
		}
		score := 0.0
		for _, n := range counts {
			score += float64(n * n)
		}
		// Strictly greater keeps the smallest-magnitude angle on ties, since
		// the loop sweeps outward symmetrically from the negative side.
		if score > bestScore || (score == bestScore && math.Abs(theta) < math.Abs(best)) {
			best, bestScore = theta, score
		}
	}
	return best
}

// groupRows clusters the deskewed words into rows and joins each left-to-right.
func groupRows(pw []positionedWord, medianH int) string {
	sort.SliceStable(pw, func(i, j int) bool { return pw[i].ry < pw[j].ry })
	threshold := float64(medianH) * 0.7
	if threshold < 1 {
		threshold = 1
	}

	var lines []string
	row := []positionedWord{pw[0]}
	anchor := pw[0].ry
	flush := func() {
		sort.SliceStable(row, func(i, j int) bool { return row[i].rx < row[j].rx })
		parts := make([]string, len(row))
		for i, p := range row {
			parts[i] = p.text
		}
		lines = append(lines, joinRow(parts))
	}
	for _, p := range pw[1:] {
		if math.Abs(p.ry-anchor) <= threshold {
			row = append(row, p)
			continue
		}
		flush()
		row = []positionedWord{p}
		anchor = p.ry
	}
	flush()
	return strings.Join(lines, "\n")
}

// joinRow concatenates a row's words, attaching punctuation tightly instead of
// always inserting a space. Vision tokenises "/", "-" and "." as separate
// words, so a naive space-join yields "B - 16", "001 / 009", "NO . 19"; this
// produces "B-16", "001/009", "NO. 19".
func joinRow(tokens []string) string {
	var b strings.Builder
	prev := ""
	for _, t := range tokens {
		if t == "" {
			continue
		}
		if prev != "" && needsSpace(prev, t) {
			b.WriteByte(' ')
		}
		b.WriteString(t)
		prev = t
	}
	return b.String()
}

func needsSpace(prev, cur string) bool {
	const attachLeading = ".,:;/-')]" // cur starting with one of these joins prev
	const attachTrailing = "/-(['"    // prev ending with one of these joins cur
	cr := []rune(cur)
	if strings.ContainsRune(attachLeading, cr[0]) {
		return false
	}
	pr := []rune(prev)
	return !strings.ContainsRune(attachTrailing, pr[len(pr)-1])
}

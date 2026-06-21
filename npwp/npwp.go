// Package npwp parses Indonesian NPWP (Nomor Pokok Wajib Pajak / tax ID) cards
// from OCR output into a typed, validated struct.
package npwp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/reynaldio/id-ocr/internal/normalize"
	"github.com/reynaldio/id-ocr/ocr"
)

// NPWP is a parsed Indonesian tax card.
type NPWP struct {
	Number    string    // 15-digit number, formatted XX.XXX.XXX.X-XXX.XXX
	NIK       string    // 16-digit NIK (present on cards issued after the 2024 unification)
	Name      string    // registered name
	Address   string    // registered address
	KPP       string    // Kantor Pelayanan Pajak (tax office), if printed
	IssueDate time.Time // registration date ("Terdaftar"), printed below the KPP
	RawText   string    `json:"-"`
}

// MarshalJSON renders IssueDate as a date-only "YYYY-MM-DD" string (null when
// unset), while the Go field stays a time.Time.
func (n NPWP) MarshalJSON() ([]byte, error) {
	type alias NPWP // strips this method, avoiding infinite recursion
	var issue *string
	if !n.IssueDate.IsZero() {
		s := n.IssueDate.Format("2006-01-02")
		issue = &s
	}
	return json.Marshal(struct {
		alias
		IssueDate *string `json:"IssueDate"`
	}{
		alias:     alias(n),
		IssueDate: issue,
	})
}

var (
	// 15-digit NPWP, with or without the usual punctuation.
	npwpRe = regexp.MustCompile(`\d{2}[.\s]?\d{3}[.\s]?\d{3}[.\s]?\d[-\s]?\d{3}[.\s]?\d{3}`)
	nikRe  = regexp.MustCompile(`\d[\d\s]{14,}\d`)
)

// Parse builds an NPWP from an OCR result. Best-effort: unreadable fields are
// left zero. Call NPWP.Validate to check the result.
func Parse(res *ocr.Result) (*NPWP, error) {
	n := &NPWP{RawText: res.Text}
	// numberSeen gates the unlabeled Name/Address heuristic: on a real card the
	// holder's name and address are printed without labels, below the NPWP
	// number and above the KPP line.
	numberSeen := false
	for _, raw := range res.Lines() {
		line := normalize.Spaces(raw)
		if isBoilerplate(line) {
			continue
		}
		if n.scanNumbers(line) {
			numberSeen = numberSeen || n.Number != ""
			continue
		}
		if n.scanLabels(line) {
			continue
		}
		if numberSeen {
			n.scanUnlabeled(line)
		}
	}
	return n, nil
}

// isBoilerplate skips the fixed institutional header lines.
func isBoilerplate(line string) bool {
	return hasPrefix(line, "kementerian") || hasPrefix(line, "direktorat") ||
		hasPrefix(line, "republik")
}

// scanUnlabeled captures the Name (first qualifying line after the number) and
// then accumulates Address lines, until a "Terdaftar" / labelled line ends the
// block. It is heuristic and only used when the labelled scan found nothing.
func (n *NPWP) scanUnlabeled(line string) {
	if hasPrefix(line, "terdaftar") {
		return
	}
	if n.Name == "" {
		n.Name = line
		return
	}
	if n.Address == "" {
		n.Address = line
	} else {
		n.Address += " " + line
	}
}

// scanNumbers extracts the NIK and NPWP number from a line. The 16-digit NIK
// is tried first: an undecorated NIK can otherwise have its first 15 digits
// mistaken for an NPWP number. It returns true if the line was consumed.
func (n *NPWP) scanNumbers(line string) bool {
	if n.NIK == "" {
		if d := normalize.Digits(nikRe.FindString(line)); len(d) == 16 {
			n.NIK = d
			return true
		}
	}
	if n.Number == "" {
		if d := normalize.Digits(npwpRe.FindString(line)); len(d) == 15 {
			n.Number = Format(d)
			return true
		}
	}
	return false
}

// scanLabels fills text fields from "Label : value" lines, returning true if
// the line matched a label.
func (n *NPWP) scanLabels(line string) bool {
	switch {
	case n.Name == "" && hasPrefix(line, "nama"):
		n.Name = normalize.AfterLabel(line)
	case n.KPP == "" && hasPrefix(line, "kpp"):
		n.KPP = normalize.AfterLabel(line)
	case n.Address == "" && hasPrefix(line, "alamat"):
		n.Address = normalize.AfterLabel(line)
	case n.IssueDate.IsZero() && hasPrefix(line, "terdaftar"):
		// "Terdaftar : 21 Agustus 2008" (printed below the KPP).
		if d, err := normalize.ParseDate(normalize.AfterLabel(line)); err == nil {
			n.IssueDate = d
		}
	default:
		return false
	}
	return true
}

// Format renders 15 raw digits in the canonical XX.XXX.XXX.X-XXX.XXX layout.
// Input that is not 15 digits is returned unchanged.
func Format(digits string) string {
	if len(digits) != 15 {
		return digits
	}
	return fmt.Sprintf("%s.%s.%s.%s-%s.%s",
		digits[0:2], digits[2:5], digits[5:8], digits[8:9], digits[9:12], digits[12:15])
}

// Validate checks that the NPWP number is structurally sound (15 digits) and,
// when present, that the NIK is 16 digits.
func (n NPWP) Validate() error {
	d := normalize.Digits(n.Number)
	if len(d) != 15 {
		return fmt.Errorf("npwp: number must be 15 digits, got %d", len(d))
	}
	if n.NIK != "" && len(n.NIK) != 16 {
		return fmt.Errorf("npwp: NIK must be 16 digits, got %d", len(n.NIK))
	}
	return nil
}

func hasPrefix(line, label string) bool {
	return strings.HasPrefix(strings.ToLower(line), label)
}

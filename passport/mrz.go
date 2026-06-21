package passport

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/reynaldio/id-ocr/idtype"
	"github.com/reynaldio/id-ocr/ocr"
)

// Parse decodes a passport from an OCR result. The TD3 MRZ is the primary,
// check-digit-verified source; the printed visual zone fills in whatever the
// MRZ lacks (issue date, place of birth) and serves as a fallback when the MRZ
// cannot be read. An error is returned only when neither yields any field.
func Parse(res *ocr.Result) (*Passport, error) {
	lines := res.Lines()
	p := &Passport{RawText: res.Text}

	if l1, l2, ok := findMRZ(lines); ok {
		if mrz, err := ParseMRZ(l1, l2); err == nil {
			mrz.RawText = res.Text
			p = mrz
		}
	}

	p.parseVisualZone(lines) // RegistrationNumber, IssuingOffice
	p.fillFromVisual(lines)  // Number/Name/DOB/Sex/Place/Issue/Expiry, if still empty

	if p.MRZ == "" && p.Number == "" && p.GivenNames == "" && p.Surname == "" {
		return nil, fmt.Errorf("passport: no MRZ or readable visual fields found")
	}
	return p, nil
}

func sameDate(a, b time.Time) bool { return !b.IsZero() && a.Equal(b) }

var noRegRe = regexp.MustCompile(`(?i)no\.?\s*reg`)

// parseVisualZone fills RegistrationNumber and IssuingOffice from the printed
// "No. Reg / Issuing Office" block, which is not part of the MRZ. The value
// usually sits on the line below the label, as "<reg-number> <office>".
func (p *Passport) parseVisualZone(lines []string) {
	for i, line := range lines {
		if !noRegRe.MatchString(line) {
			continue
		}
		// The value can be on the label line or a few lines below it — an
		// "ISSUING OFFICE" sub-label often sits in between.
		for k := i; k < i+4 && k < len(lines); k++ {
			if reg, office, ok := splitRegLine(lines[k]); ok {
				p.RegistrationNumber, p.IssuingOffice = reg, office
				return
			}
		}
	}
}

// splitRegLine reads "<reg-number> <issuing office>" from a line: the first
// token must look like a registration number (alphanumeric with both letters
// and digits), and the remainder is the office.
func splitRegLine(line string) (reg, office string, ok bool) {
	for i, f := range strings.Fields(line) {
		u := strings.ToUpper(f)
		if !isRegNumber(u) {
			continue
		}
		rest := strings.Fields(line)[i+1:]
		return u, strings.Join(rest, " "), true
	}
	return "", "", false
}

func isRegNumber(s string) bool {
	if len(s) < 6 {
		return false
	}
	var hasDigit, hasLetter bool
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r >= 'A' && r <= 'Z':
			hasLetter = true
		default:
			return false
		}
	}
	return hasDigit && hasLetter
}

// findMRZ locates the two TD3 MRZ rows in OCR output. It first looks for two
// consecutive full-ish lines; failing that (some engines split each 44-char row
// into several pieces) it reassembles the rows from fragments, using the MRZ
// check digits to confirm the alignment.
func findMRZ(lines []string) (string, string, bool) {
	norm := make([]string, 0, len(lines))
	for _, l := range lines {
		if n := normalizeMRZ(l); n != "" {
			norm = append(norm, n)
		}
	}
	for i := 0; i+1 < len(norm); i++ {
		if a, ok := mrzLine(norm[i]); ok {
			if b, ok := mrzLine(norm[i+1]); ok {
				return a, b, true
			}
		}
	}
	return reassembleMRZ(norm)
}

// reassembleMRZ rebuilds the two 44-char rows from a run of fragments. Starting
// at each fragment it concatenates following fragments to 88 characters, splits
// 44/44, and accepts the split only when the MRZ check digits validate — so
// interleaved non-MRZ text simply fails to validate rather than corrupting the
// result.
func reassembleMRZ(frags []string) (string, string, bool) {
	for i := range frags {
		var b strings.Builder
		for j := i; j < len(frags) && b.Len() < 88; j++ {
			b.WriteString(frags[j])
		}
		s := b.String()
		if len(s) < 88 {
			continue
		}
		l1, l2 := s[:44], s[44:88]
		if p, err := ParseMRZ(l1, l2); err == nil &&
			p.Checks.Composite && p.Checks.BirthDate && p.Checks.ExpiryDate {
			return l1, l2, true
		}
	}
	return "", "", false
}

// mrzLine reports whether n looks like a TD3 MRZ row and returns it normalised
// to exactly 44 characters. OCR engines (notably Google Vision) commonly drop
// trailing "<" fillers, so a slightly short candidate is right-padded.
func mrzLine(n string) (string, bool) {
	if len(n) < 30 || len(n) > 44 || !strings.ContainsRune(n, '<') {
		return "", false
	}
	if len(n) < 44 {
		n += strings.Repeat("<", 44-len(n))
	}
	return n, true
}

// normalizeMRZ upper-cases and keeps only the MRZ alphabet [A-Z0-9<], dropping
// spaces. Vision inserts spurious spaces between MRZ tokens; since the zone
// contains no real spaces (positions are padded with "<"), they are removed
// rather than treated as fillers.
func normalizeMRZ(s string) string {
	s = strings.ToUpper(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '<':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ParseMRZ decodes two already-normalised 44-character TD3 MRZ lines.
//
// Line 1: P<ISSGIVENNAMES<<SURNAME...  (positions 0-43)
// Line 2: NUMBER(9) c NAT(3) DOB(6) c SEX(1) EXP(6) c PERSONAL(14) c COMPOSITE(1)
func ParseMRZ(line1, line2 string) (*Passport, error) {
	if len(line1) != 44 || len(line2) != 44 {
		return nil, fmt.Errorf("passport: TD3 MRZ lines must be 44 chars (got %d, %d)", len(line1), len(line2))
	}
	p := &Passport{MRZ: line1 + "\n" + line2}

	// --- Line 1: type, issuer, names ---
	p.Type = strings.TrimRight(line1[0:1], "<")
	p.IssuingCountry = strings.TrimRight(line1[2:5], "<")
	p.Surname, p.GivenNames = splitNames(line1[5:44])

	// --- Line 2: number, nationality, dob, sex, expiry, personal ---
	number := line2[0:9]
	numCheck := line2[9]
	p.Nationality = strings.TrimRight(line2[10:13], "<")
	dob := line2[13:19]
	dobCheck := line2[19]
	sex := line2[20]
	exp := line2[21:27]
	expCheck := line2[27]
	personal := line2[28:42]
	personalCheck := line2[42]
	composite := line2[43]

	p.Number = strings.TrimRight(number, "<")
	p.PersonalNumber = strings.TrimRight(personal, "<")
	p.Sex = decodeSex(sex)
	if d, ok := decodeDate(dob, false); ok {
		p.BirthDate = d
	}
	if d, ok := decodeDate(exp, true); ok {
		p.ExpiryDate = d
	}

	// --- Check digits ---
	p.Checks.Number = checkDigit(number) == numCheck
	p.Checks.BirthDate = checkDigit(dob) == dobCheck
	p.Checks.ExpiryDate = checkDigit(exp) == expCheck
	// Per ICAO 9303, a personal-number field that is entirely fillers may use
	// a filler ('<') or '0' check digit.
	if strings.Trim(personal, "<") == "" {
		p.Checks.Personal = personalCheck == '<' || personalCheck == '0'
	} else {
		p.Checks.Personal = checkDigit(personal) == personalCheck
	}
	compInput := number + string(numCheck) + dob + string(dobCheck) + exp + string(expCheck) + personal + string(personalCheck)
	p.Checks.Composite = checkDigit(compInput) == composite

	return p, nil
}

func splitNames(field string) (surname, given string) {
	parts := strings.SplitN(field, "<<", 2)
	surname = strings.TrimRight(parts[0], "<")
	surname = strings.ReplaceAll(surname, "<", " ")
	if len(parts) == 2 {
		given = strings.TrimRight(parts[1], "<")
		given = strings.ReplaceAll(given, "<", " ")
		given = strings.Join(strings.Fields(given), " ")
	}
	return surname, given
}

func decodeSex(b byte) idtype.Sex {
	switch b {
	case 'M':
		return idtype.Male
	case 'F':
		return idtype.Female
	case 'X', '<':
		return idtype.Unspecified
	default:
		return ""
	}
}

// decodeDate parses a 6-char YYMMDD MRZ date. expiry pivots two-digit years
// forward (00-69 -> 2000s); birth dates pivot around the current year.
func decodeDate(s string, expiry bool) (time.Time, bool) {
	if len(s) != 6 {
		return time.Time{}, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return time.Time{}, false
		}
	}
	yy := atoi(s[0:2])
	mm := atoi(s[2:4])
	dd := atoi(s[4:6])
	if mm < 1 || mm > 12 || dd < 1 || dd > 31 {
		return time.Time{}, false
	}
	var year int
	if expiry {
		year = 2000 + yy
	} else {
		now := time.Now().UTC().Year()
		year = 2000 + yy
		if year > now {
			year -= 100
		}
	}
	t := time.Date(year, time.Month(mm), dd, 0, 0, 0, 0, time.UTC)
	if t.Day() != dd || int(t.Month()) != mm {
		return time.Time{}, false
	}
	return t, true
}

// checkDigit computes the ICAO 9303 check digit over s using the repeating
// 7-3-1 weighting. Letters are valued A=10..Z=35, '<' and fillers are 0.
func checkDigit(s string) byte {
	weights := [3]int{7, 3, 1}
	sum := 0
	for i := 0; i < len(s); i++ {
		sum += charValue(s[i]) * weights[i%3]
	}
	return byte('0' + sum%10)
}

func charValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	default: // '<' and anything else
		return 0
	}
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

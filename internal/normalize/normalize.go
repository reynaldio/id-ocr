// Package normalize holds shared text and value cleanup helpers used by the
// document parsers: whitespace collapsing, label stripping, and Indonesian
// date parsing.
package normalize

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var spaceRe = regexp.MustCompile(`\s+`)

// Spaces collapses runs of whitespace into a single space and trims the ends.
func Spaces(s string) string {
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// AfterLabel returns the value portion of a "Label : value" style line. It is
// tolerant of OCR noise: the separator may be ":", missing, or surrounded by
// stray spaces. If sep is not found, the whole cleaned line is returned.
func AfterLabel(line string) string {
	if i := strings.IndexAny(line, ":"); i >= 0 {
		return Spaces(line[i+1:])
	}
	return Spaces(line)
}

// Digits returns only the ASCII digits in s. Useful for pulling NIK/NPWP
// numbers out of noisy lines.
func Digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteByte(byte(r))
		}
	}
	return b.String()
}

var indoMonths = map[string]time.Month{
	"januari": time.January, "februari": time.February, "maret": time.March,
	"april": time.April, "mei": time.May, "juni": time.June,
	"juli": time.July, "agustus": time.August, "september": time.September,
	"oktober": time.October, "november": time.November, "desember": time.December,
	// common OCR/abbreviation variants
	"jan": time.January, "feb": time.February, "mar": time.March, "apr": time.April,
	"jun": time.June, "jul": time.July, "agu": time.August, "agt": time.August,
	"sep": time.September, "okt": time.October, "nov": time.November, "des": time.December,
	// English month abbreviations (passport visual zone, e.g. "04 OCT 2022")
	"may": time.May, "aug": time.August, "oct": time.October, "dec": time.December,
	"sept": time.September,
}

var numericDateRe = regexp.MustCompile(`(\d{1,2})\s*[-/.]\s*(\d{1,2})\s*[-/.]\s*(\d{2,4})`)
var wordDateRe = regexp.MustCompile(`(\d{1,2})\s+([A-Za-z]+)\s+(\d{4})`)

// ParseDate parses Indonesian ID date formats: numeric "DD-MM-YYYY"
// (also "/" or "." separators) and worded "DD MONTH YYYY" (Indonesian month
// names). Two-digit years are pivoted around 1970-2069.
func ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if m := numericDateRe.FindStringSubmatch(s); m != nil {
		return assemble(m[1], 0, m[2], m[3])
	}
	if m := wordDateRe.FindStringSubmatch(s); m != nil {
		mon, ok := indoMonths[strings.ToLower(m[2])]
		if !ok {
			return time.Time{}, fmt.Errorf("normalize: unknown month %q", m[2])
		}
		return assemble(m[1], mon, "", m[3])
	}
	return time.Time{}, fmt.Errorf("normalize: cannot parse date %q", s)
}

func assemble(dayS string, monthName time.Month, monthS, yearS string) (time.Time, error) {
	day := atoi(dayS)
	month := int(monthName)
	if monthS != "" {
		month = atoi(monthS)
	}
	year := atoi(yearS)
	if len(yearS) == 2 {
		if year < 70 {
			year += 2000
		} else {
			year += 1900
		}
	}
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("normalize: out-of-range date %d-%d-%d", year, month, day)
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	// Reject overflow (e.g. 31 Feb rolling into March).
	if t.Day() != day || int(t.Month()) != month {
		return time.Time{}, fmt.Errorf("normalize: invalid calendar date %d-%d-%d", year, month, day)
	}
	return t, nil
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			continue
		}
		n = n*10 + int(r-'0')
	}
	return n
}

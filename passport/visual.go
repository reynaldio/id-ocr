package passport

import (
	"regexp"
	"strings"
	"time"

	"github.com/reynaldio/id-ocr/idtype"
	"github.com/reynaldio/id-ocr/internal/normalize"
)

var (
	passportNumRe = regexp.MustCompile(`\b[A-Z]\d{6,8}\b`)
	dateTokenRe   = regexp.MustCompile(`\d{1,2}\s+[A-Za-z]{3,}\s+\d{4}|\d{1,2}[-/.]\d{1,2}[-/.]\d{2,4}`)
	countryCodeRe = regexp.MustCompile(`^[A-Z]{2,3}$`)
)

// fillFromVisual populates fields still empty after MRZ parsing from the printed
// visual zone. It runs for every passport: when the MRZ parsed, it only adds
// what the MRZ lacks (issue date, place of birth); when the MRZ is missing or
// incomplete it recovers the core fields too. Visual-zone values are best-effort
// and NOT check-digit protected — Checks reports the MRZ verification status.
func (p *Passport) fillFromVisual(lines []string) {
	for i, line := range lines {
		l := strings.TrimSpace(line)
		u := strings.ToUpper(l)

		if p.GivenNames == "" && (strings.Contains(u, "FULL NAME") || strings.Contains(u, "NAMA LENGKAP")) {
			p.GivenNames = nameAfter(lines, i)
			continue
		}
		switch dates := findDates(l); {
		case len(dates) >= 2: // the "issue   expiry" row
			p.setIssueExpiry(dates)
		case len(dates) == 1 && sexMarker(u) != "": // the "DOB sex place" row
			p.fillBirthRow(l, dates[0])
		}
	}

	if p.Number == "" {
		p.Number = passportNumRe.FindString(strings.ToUpper(strings.Join(lines, " ")))
	}
	if p.IssueDate.IsZero() {
		p.findIssueDate(lines)
	}
	p.fillCountry(lines)
}

func (p *Passport) fillBirthRow(line string, dob time.Time) {
	if p.BirthDate.IsZero() {
		p.BirthDate = dob
	}
	if p.Sex == "" {
		p.Sex = sexMarker(strings.ToUpper(line))
	}
	if p.BirthPlace == "" {
		p.BirthPlace = placeAfterSex(line)
	}
}

func (p *Passport) setIssueExpiry(dates []time.Time) {
	lo, hi := dates[0], dates[0]
	for _, d := range dates {
		if d.Before(lo) {
			lo = d
		}
		if d.After(hi) {
			hi = d
		}
	}
	if p.IssueDate.IsZero() {
		p.IssueDate = lo
	}
	if p.ExpiryDate.IsZero() {
		p.ExpiryDate = hi
	}
}

// findIssueDate sets IssueDate to the first date that is neither the birth nor
// the expiry date (passports print the issue date on its own when not paired).
func (p *Passport) findIssueDate(lines []string) {
	for _, line := range lines {
		for _, d := range findDates(line) {
			if sameDate(d, p.BirthDate) || sameDate(d, p.ExpiryDate) {
				continue
			}
			p.IssueDate = d
			return
		}
	}
}

// fillCountry sets Type/IssuingCountry/Nationality for an Indonesian passport
// when the MRZ did not supply them.
func (p *Passport) fillCountry(lines []string) {
	if p.Number == "" {
		return // nothing recognised; don't guess
	}
	if p.Type == "" {
		p.Type = "P"
	}
	for _, line := range lines {
		u := strings.ToUpper(line)
		if strings.Contains(u, "IDN") || strings.Contains(u, "INDONESIA") {
			if p.IssuingCountry == "" {
				p.IssuingCountry = "IDN"
			}
			if p.Nationality == "" {
				p.Nationality = "IDN"
			}
			return
		}
	}
}

func findDates(line string) []time.Time {
	var out []time.Time
	for _, m := range dateTokenRe.FindAllString(line, -1) {
		if d, err := normalize.ParseDate(m); err == nil {
			out = append(out, d)
		}
	}
	return out
}

// sexMarker reads the "<indo>/<eng>" sex cell printed on the data page, e.g.
// "L/M" (male) or "P/F" (female).
func sexMarker(u string) idtype.Sex {
	switch {
	case strings.Contains(u, "/F"):
		return idtype.Female
	case strings.Contains(u, "/M"):
		return idtype.Male
	}
	return ""
}

func placeAfterSex(line string) string {
	fields := strings.Fields(line)
	for i, f := range fields {
		uf := strings.ToUpper(f)
		if (strings.Contains(uf, "/F") || strings.Contains(uf, "/M")) && i+1 < len(fields) {
			return strings.ToUpper(strings.Join(fields[i+1:], " "))
		}
	}
	return ""
}

// nameAfter returns the holder's name printed below a "Full Name" label,
// skipping a stray country-code line (e.g. "IDN") that often precedes it.
func nameAfter(lines []string, i int) string {
	for j := i + 1; j < len(lines) && j <= i+3; j++ {
		v := strings.TrimSpace(lines[j])
		u := strings.ToUpper(v)
		if v == "" || countryCodeRe.MatchString(u) {
			continue
		}
		if strings.Contains(u, "NATIONALITY") || strings.Contains(u, "KEWARGANEGARAAN") {
			break
		}
		return normalize.Spaces(u)
	}
	return ""
}

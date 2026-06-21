package passport

import (
	"strings"
	"testing"

	"github.com/reynaldio/id-ocr/idtype"
	"github.com/reynaldio/id-ocr/ocr"
)

// The canonical ICAO 9303 TD3 specimen, whose check digits are all valid.
const (
	specimenL1 = "P<UTOERIKSSON<<ANNA<MARIA<<<<<<<<<<<<<<<<<<<"
	specimenL2 = "L898902C36UTO7408122F1204159ZE184226B<<<<<10"
)

// spaceOut inserts a space every n characters, mimicking how Google Vision
// tokenises the MRZ into separate words.
func spaceOut(s string, n int) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%n == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func TestParse_VisionStyleMRZ(t *testing.T) {
	// Vision-style: spaces between tokens, and line 1 missing 4 trailing "<"
	// fillers (which findMRZ pads back). Surrounding page text must be ignored.
	l1 := spaceOut(specimenL1[:40], 6) // drop 4 trailing fillers
	l2 := spaceOut(specimenL2, 8)
	text := "REPUBLIK INDONESIA\nPASSPORT\n" + l1 + "\n" + l2 + "\nREPUBLIK IND"

	p, err := Parse(&ocr.Result{Text: text})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Surname != "ERIKSSON" || p.GivenNames != "ANNA MARIA" {
		t.Errorf("names = %q / %q", p.Surname, p.GivenNames)
	}
	if p.Number != "L898902C3" {
		t.Errorf("number = %q", p.Number)
	}
	if !p.Checks.Valid() {
		t.Errorf("check digits failed: %+v", p.Checks)
	}
}

func TestParse_VisualZone(t *testing.T) {
	// The "No. Reg / Issuing Office" block sits in the printed visual zone,
	// above the MRZ; the value is on the line below the label.
	// Issue date is in the visual zone only (specimen MRZ: born 1974-08-12,
	// expires 2012-04-15), so a different printed date is the issue date.
	text := "DATE OF ISSUE 10 MAR 2007\n" +
		"NO.REG ISSUING MENGELUARKAN OFFICE\n" +
		"1X23YZ4567ABCD BOGOR\n" +
		spaceOut(specimenL1[:40], 6) + "\n" + spaceOut(specimenL2, 8)

	p, err := Parse(&ocr.Result{Text: text})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.RegistrationNumber != "1X23YZ4567ABCD" {
		t.Errorf("RegistrationNumber = %q", p.RegistrationNumber)
	}
	if p.IssuingOffice != "BOGOR" {
		t.Errorf("IssuingOffice = %q", p.IssuingOffice)
	}
	if got := p.IssueDate.Format("2006-01-02"); got != "2007-03-10" {
		t.Errorf("IssueDate = %s, want 2007-03-10", got)
	}
}

func TestParseMRZ_Specimen(t *testing.T) {
	p, err := ParseMRZ(specimenL1, specimenL2)
	if err != nil {
		t.Fatalf("ParseMRZ: %v", err)
	}
	if p.Type != "P" {
		t.Errorf("Type = %q, want P", p.Type)
	}
	if p.IssuingCountry != "UTO" {
		t.Errorf("IssuingCountry = %q, want UTO", p.IssuingCountry)
	}
	if p.Surname != "ERIKSSON" {
		t.Errorf("Surname = %q, want ERIKSSON", p.Surname)
	}
	if p.GivenNames != "ANNA MARIA" {
		t.Errorf("GivenNames = %q, want ANNA MARIA", p.GivenNames)
	}
	if p.Number != "L898902C3" {
		t.Errorf("Number = %q, want L898902C3", p.Number)
	}
	if p.Sex != idtype.Female {
		t.Errorf("Sex = %q, want F", p.Sex)
	}
	if got := p.BirthDate.Format("2006-01-02"); got != "1974-08-12" {
		t.Errorf("BirthDate = %s, want 1974-08-12", got)
	}
	if got := p.ExpiryDate.Format("2006-01-02"); got != "2012-04-15" {
		t.Errorf("ExpiryDate = %s, want 2012-04-15", got)
	}
	if !p.Checks.Valid() {
		t.Errorf("check digits failed: %+v", p.Checks)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestParseMRZ_BadCheckDigit(t *testing.T) {
	// Corrupt the document-number check digit (position 9).
	bad := []byte(specimenL2)
	bad[9] = '0'
	p, err := ParseMRZ(specimenL1, string(bad))
	if err != nil {
		t.Fatalf("ParseMRZ: %v", err)
	}
	if p.Checks.Number {
		t.Error("expected document-number check to fail")
	}
	if err := p.Validate(); err == nil {
		t.Error("expected Validate to report failure")
	}
}

func TestCheckDigit(t *testing.T) {
	// Worked example from ICAO 9303 part 3.
	if got := checkDigit("D23145890734"); got != '9' {
		t.Errorf("checkDigit = %c, want 9", got)
	}
}

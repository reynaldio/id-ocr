package passport

import (
	"strings"
	"testing"

	"github.com/reynaldio/id-ocr/idtype"
	"github.com/reynaldio/id-ocr/ocr"
)

func TestParse_VisualFallback(t *testing.T) {
	// No MRZ at all — only the printed visual zone, mimicking a passport whose
	// MRZ could not be read.
	text := strings.Join([]string{
		"REPUBLIK INDONESIA",
		"P IDN A1234567",
		"NAMA LENGKAP/FULL NAME",
		"IDN",
		"SITI AMINAH",
		"KEWARGANEGARAAN/NATIONALITY",
		"INDONESIA",
		"TGL LAHIR/DATE OF BIRTH KELAMIN/SEX TEMPAT LAHIR/PLACE OF BIRTH",
		"23 FEB 1997 P/F BOGOR",
		"TGL PENGELUARAN/DATE OF ISSUE TGL HABIS BERLAKU DATE OF EXPIRY",
		"15 AUG 2022 15 AUG 2027",
		"NO.REG. KANTOR YANG MENGELUARKAN/ ISSUING OFFICE",
		"1X23YZ4567ABCD BOGOR",
	}, "\n")

	p, err := Parse(&ocr.Result{Text: text})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	str := map[string]struct{ got, want string }{
		"Number":             {p.Number, "A1234567"},
		"GivenNames":         {p.GivenNames, "SITI AMINAH"},
		"BirthPlace":         {p.BirthPlace, "BOGOR"},
		"Nationality":        {p.Nationality, "IDN"},
		"RegistrationNumber": {p.RegistrationNumber, "1X23YZ4567ABCD"},
		"IssuingOffice":      {p.IssuingOffice, "BOGOR"},
	}
	for f, c := range str {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", f, c.got, c.want)
		}
	}
	if p.Sex != idtype.Female {
		t.Errorf("Sex = %q, want FEMALE", p.Sex)
	}
	for name, got := range map[string]string{
		"BirthDate":  p.BirthDate.Format("2006-01-02"),
		"IssueDate":  p.IssueDate.Format("2006-01-02"),
		"ExpiryDate": p.ExpiryDate.Format("2006-01-02"),
	} {
		want := map[string]string{"BirthDate": "1997-02-23", "IssueDate": "2022-08-15", "ExpiryDate": "2027-08-15"}[name]
		if got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}

	// Visual-only data is unverified: no MRZ check digits, Validate fails.
	if p.Checks.Valid() {
		t.Error("expected Checks to be invalid for visual-only data")
	}
	if err := p.Validate(); err == nil {
		t.Error("expected Validate to fail without a verified MRZ")
	}
}

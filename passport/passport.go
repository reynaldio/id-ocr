// Package passport parses passports (Indonesian and other ICAO TD3 booklets)
// from OCR output, focusing on the machine-readable zone (MRZ).
package passport

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/reynaldio/id-ocr/idtype"
)

// Passport holds the fields decoded from a TD3 machine-readable passport.
type Passport struct {
	Type           string     // document type, usually "P"
	IssuingCountry string     // 3-letter ISO country code (e.g. "IDN")
	Surname        string     // primary identifier
	GivenNames     string     // secondary identifier
	Number         string     // passport number
	Nationality    string     // 3-letter ISO country code
	BirthDate      time.Time  // date of birth
	BirthPlace     string     // place of birth — visual zone only (not in the MRZ)
	Sex            idtype.Sex // normalized to MALE/FEMALE/UNSPECIFIED
	IssueDate      time.Time  // date of issue — visual zone only (not in the MRZ)
	ExpiryDate     time.Time  // date of expiry
	PersonalNumber string     // MRZ optional-data field (line 2, positions 28-41); often a form of the NIK

	// RegistrationNumber ("No. Reg") and IssuingOffice are read from the
	// printed visual zone, not the MRZ, so they are NOT protected by check
	// digits and depend on visual-text OCR quality.
	RegistrationNumber string
	IssuingOffice      string

	MRZ     string // the two raw MRZ lines, newline-separated
	RawText string `json:"-"`

	// Checks reports per-field MRZ check-digit validation.
	Checks Checks
}

// MarshalJSON renders BirthDate and ExpiryDate as date-only "YYYY-MM-DD"
// strings (null when unset), while the Go fields stay time.Time.
func (p Passport) MarshalJSON() ([]byte, error) {
	type alias Passport // strips this method, avoiding infinite recursion
	return json.Marshal(struct {
		alias
		BirthDate  *string `json:"BirthDate"`
		IssueDate  *string `json:"IssueDate"`
		ExpiryDate *string `json:"ExpiryDate"`
	}{
		alias:      alias(p),
		BirthDate:  dateOnly(p.BirthDate),
		IssueDate:  dateOnly(p.IssueDate),
		ExpiryDate: dateOnly(p.ExpiryDate),
	})
}

func dateOnly(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

// Checks records which MRZ check digits validated.
type Checks struct {
	Number     bool
	BirthDate  bool
	ExpiryDate bool
	Personal   bool
	Composite  bool
}

// Valid reports whether every MRZ check digit validated.
func (c Checks) Valid() bool {
	return c.Number && c.BirthDate && c.ExpiryDate && c.Personal && c.Composite
}

// Validate returns an error if any MRZ check digit failed.
func (p Passport) Validate() error {
	if !p.Checks.Valid() {
		return fmt.Errorf("passport: MRZ check digits failed: %+v", p.Checks)
	}
	return nil
}

// Package ktp parses Indonesian KTP (Kartu Tanda Penduduk) e-ID cards from
// OCR output into a typed, validated struct.
package ktp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/reynaldio/id-ocr/idtype"
)

// LifetimeDate is the sentinel ValidUntilDate used when a card never expires
// ("Berlaku Hingga: SEUMUR HIDUP"). It is the maximum calendar date (9999-12-31,
// the max accepted by SQL DATE and most systems) rather than a zero time, so
// date comparisons treat a lifetime card as "not expired".
var LifetimeDate = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

// KTP is a parsed Indonesian identity card.
type KTP struct {
	NIK            string     // 16-digit national identity number
	Name           string     // Nama
	BirthPlace     string     // Tempat Lahir
	BirthDate      time.Time  // Tanggal Lahir (zero if unparseable)
	Sex            idtype.Sex // Jenis Kelamin, normalized to MALE/FEMALE
	BloodType      string     // Golongan Darah (e.g. "O", "A", "-")
	Address        string     // Alamat
	RTRW           string     // RT/RW
	Village        string     // Kelurahan/Desa
	District       string     // Kecamatan
	Religion       string     // Agama
	MaritalStatus  string     // Status Perkawinan
	Occupation     string     // Pekerjaan
	Nationality    string     // Kewarganegaraan
	Province       string     // Provinsi
	Regency        string     // Kabupaten/Kota
	IssuingCity    string     // city of issue (bottom-right); equals the Kabupaten/Kota name
	IssuingDate    time.Time  // date of issue (bottom-right), zero if unparseable
	ValidUntilText string     // Berlaku Hingga as printed ("SEUMUR HIDUP" or a date)
	ValidUntilDate time.Time  // parsed ValidUntilText; LifetimeDate for "SEUMUR HIDUP"
	RawText        string     `json:"-"` // full OCR text the parse was derived from
}

// MarshalJSON renders BirthDate and ValidUntilDate as date-only "YYYY-MM-DD"
// strings (null when unset), while the Go fields stay time.Time. All other
// fields marshal as usual; RawText remains excluded.
func (k KTP) MarshalJSON() ([]byte, error) {
	type alias KTP // strips this method, avoiding infinite recursion
	return json.Marshal(struct {
		alias
		BirthDate      *string `json:"BirthDate"`
		IssuingDate    *string `json:"IssuingDate"`
		ValidUntilDate *string `json:"ValidUntilDate"`
	}{
		alias:          alias(k),
		BirthDate:      dateOnly(k.BirthDate),
		IssuingDate:    dateOnly(k.IssuingDate),
		ValidUntilDate: dateOnly(k.ValidUntilDate),
	})
}

func dateOnly(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

// Validate reports whether the parsed card is internally consistent. It checks
// the NIK and, when both are present, that the NIK's embedded birth date
// matches the parsed birth date.
func (k KTP) Validate() error {
	if err := ValidateNIK(k.NIK); err != nil {
		return err
	}
	if !k.BirthDate.IsZero() {
		if d, ok := NIKBirthDate(k.NIK); ok {
			if d.Day() != k.BirthDate.Day() || d.Month() != k.BirthDate.Month() {
				return fmt.Errorf("ktp: birth date %s does not match NIK-embedded date %s",
					k.BirthDate.Format("02-01"), d.Format("02-01"))
			}
		}
	}
	return nil
}

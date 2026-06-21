package ktp

import (
	"fmt"
	"time"

	"github.com/reynaldio/id-ocr/idtype"
)

// ValidateNIK checks the structural validity of a 16-digit NIK:
//
//	PPKKCC DDMMYY SSSS
//	│ │ │  │       └ sequence (0001..9999)
//	│ │ │  └ date of birth (DD+40 for women)
//	│ │ └ kecamatan code
//	│ └ kabupaten/kota code
//	└ provinsi code
//
// It verifies the length, that all characters are digits, that the embedded
// date is a real calendar date, and that the sequence is non-zero.
func ValidateNIK(nik string) error {
	if len(nik) != 16 {
		return fmt.Errorf("ktp: NIK must be 16 digits, got %d", len(nik))
	}
	for i, r := range nik {
		if r < '0' || r > '9' {
			return fmt.Errorf("ktp: NIK contains non-digit at position %d", i)
		}
	}
	if nik[0:6] == "000000" {
		return fmt.Errorf("ktp: NIK region code is empty")
	}
	if _, ok := NIKBirthDate(nik); !ok {
		return fmt.Errorf("ktp: NIK contains an invalid birth date")
	}
	if nik[12:16] == "0000" {
		return fmt.Errorf("ktp: NIK sequence number is zero")
	}
	return nil
}

// NIKBirthDate extracts the birth date encoded in positions 7-12 of the NIK.
// For female holders the day-of-month is offset by 40. The returned bool is
// false if the NIK is malformed or the encoded date is not a real date.
//
// The two-digit year is pivoted around the current year: a year that would be
// in the future is treated as belonging to the previous century.
func NIKBirthDate(nik string) (time.Time, bool) {
	if len(nik) != 16 {
		return time.Time{}, false
	}
	dd := atoi(nik[6:8])
	mm := atoi(nik[8:10])
	yy := atoi(nik[10:12])
	if dd > 40 { // female
		dd -= 40
	}
	year := pivotYear(yy)
	if mm < 1 || mm > 12 || dd < 1 || dd > 31 {
		return time.Time{}, false
	}
	t := time.Date(year, time.Month(mm), dd, 0, 0, 0, 0, time.UTC)
	if t.Day() != dd || int(t.Month()) != mm {
		return time.Time{}, false
	}
	return t, true
}

// SexFromNIK reports the gender encoded in the NIK (day-of-month > 40 == female).
func SexFromNIK(nik string) (idtype.Sex, bool) {
	if len(nik) != 16 {
		return "", false
	}
	if atoi(nik[6:8]) > 40 {
		return idtype.Female, true
	}
	return idtype.Male, true
}

func pivotYear(yy int) int {
	now := time.Now().UTC().Year()
	century := (now / 100) * 100
	year := century + yy
	if year > now {
		year -= 100
	}
	return year
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	return n
}

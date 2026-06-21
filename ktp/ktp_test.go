package ktp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/reynaldio/id-ocr/idtype"
	"github.com/reynaldio/id-ocr/ocr"
)

func TestKTP_MarshalJSON_DateOnly(t *testing.T) {
	k := KTP{Name: "BUDI", BirthDate: time.Date(1990, 5, 9, 0, 0, 0, 0, time.UTC), RawText: "raw"}
	b, err := json.Marshal(k)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"BirthDate":"1990-05-09"`) {
		t.Errorf("date not date-only: %s", got)
	}
	if strings.Contains(got, "RawText") {
		t.Errorf("RawText should be omitted: %s", got)
	}

	// Zero date marshals to null.
	z, _ := json.Marshal(KTP{Name: "X"})
	if !strings.Contains(string(z), `"BirthDate":null`) {
		t.Errorf("zero date should be null: %s", z)
	}
}

const sampleKTP = `PROVINSI DKI JAKARTA
KOTA JAKARTA PUSAT
NIK : 3171010905900001
Nama : BUDI SANTOSO
Tempat/Tgl Lahir : JAKARTA, 09-05-1990
Jenis Kelamin : LAKI-LAKI    Gol. Darah : O
Alamat : JL MERDEKA NO 17
RT/RW : 001/002
Kel/Desa : GAMBIR
Kecamatan : GAMBIR
Agama : ISLAM
Status Perkawinan : KAWIN
Pekerjaan : KARYAWAN SWASTA
Kewarganegaraan : WNI
Berlaku Hingga : SEUMUR HIDUP`

func TestParseKTP(t *testing.T) {
	k, err := Parse(&ocr.Result{Text: sampleKTP})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if k.NIK != "3171010905900001" {
		t.Errorf("NIK = %q", k.NIK)
	}
	if k.Name != "BUDI SANTOSO" {
		t.Errorf("Name = %q", k.Name)
	}
	if k.BirthPlace != "JAKARTA" {
		t.Errorf("BirthPlace = %q", k.BirthPlace)
	}
	if got := k.BirthDate.Format("2006-01-02"); got != "1990-05-09" {
		t.Errorf("BirthDate = %s", got)
	}
	if k.Sex != idtype.Male {
		t.Errorf("Sex = %q", k.Sex)
	}
	if k.BloodType != "O" {
		t.Errorf("BloodType = %q", k.BloodType)
	}
	if k.District != "GAMBIR" {
		t.Errorf("District = %q", k.District)
	}
	if k.Province != "DKI JAKARTA" {
		t.Errorf("Province = %q", k.Province)
	}
	if k.Regency != "KOTA JAKARTA PUSAT" {
		t.Errorf("Regency = %q", k.Regency)
	}
	if k.Village != "GAMBIR" {
		t.Errorf("Village = %q", k.Village)
	}
	if k.ValidUntilText != "SEUMUR HIDUP" {
		t.Errorf("ValidUntilText = %q", k.ValidUntilText)
	}
	if !k.ValidUntilDate.Equal(LifetimeDate) {
		t.Errorf("ValidUntilDate = %s, want %s", k.ValidUntilDate, LifetimeDate)
	}
	if err := k.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// visionKTP mimics Google Vision's geometry-reconstructed output: a wrapped
// (two-line) address, and issuing-block text bleeding onto field rows. All
// values are fictional.
const visionKTP = `PROVINSI JAWA BARAT
KABUPATEN BOGOR
NIK : 3201015708950002
Nama : SITI AMINAH
Tempat/Tgl Lahir : BOGOR, 17-08-1995
Jenis kelamin : PEREMPUAN Gol. Darah: -
Alamat : JALAN CONTOH RAYA BLOK
A-1 NO.2
RT/RW € 003/004
Kel/Desa SUKAMAJU
Kecamatan CIBINONG
Agama ISLAM
Status Perkawinan : BELUM KAWIN BOGOR
Pekerjaan : KARYAWAN SWASTA 10-05-2018
Kewarganegaraan : WNI
Berlaku Hingga SEUMUR HIDUP MAL`

func TestParseKTP_VisionLayout(t *testing.T) {
	k, err := Parse(&ocr.Result{Text: visionKTP})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	checks := map[string]struct{ got, want string }{
		"Address":        {k.Address, "JALAN CONTOH RAYA BLOK A-1 NO.2"},
		"RTRW":           {k.RTRW, "003/004"},
		"Village":        {k.Village, "SUKAMAJU"},
		"District":       {k.District, "CIBINONG"},
		"MaritalStatus":  {k.MaritalStatus, "BELUM KAWIN"},
		"Occupation":     {k.Occupation, "KARYAWAN SWASTA"},
		"Nationality":    {k.Nationality, "WNI"},
		"ValidUntilText": {k.ValidUntilText, "SEUMUR HIDUP"},
		"IssuingCity":    {k.IssuingCity, "BOGOR"},
	}
	for field, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", field, c.got, c.want)
		}
	}
	if got := k.IssuingDate.Format("2006-01-02"); got != "2018-05-10" {
		t.Errorf("IssuingDate = %s, want 2018-05-10", got)
	}
	if err := k.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateNIK(t *testing.T) {
	tests := []struct {
		nik     string
		wantErr bool
	}{
		{"3171234509900001", false},
		{"317123455005000 1", true}, // contains space -> wrong length
		{"31712345", true},          // too short
		{"3171234500000001", true},  // invalid embedded date (day 00)
		{"3171234509900000", true},  // zero sequence
	}
	for _, tt := range tests {
		err := ValidateNIK(tt.nik)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateNIK(%q) err=%v, wantErr=%v", tt.nik, err, tt.wantErr)
		}
	}
}

func TestSexFromNIK(t *testing.T) {
	// day-of-month 49 -> 49-40=9, female.
	if s, _ := SexFromNIK("3171234909900001"); s != idtype.Female {
		t.Errorf("expected Female, got %q", s)
	}
	if s, _ := SexFromNIK("3171234909900001"); s == idtype.Male {
		t.Errorf("unexpected Male")
	}
}

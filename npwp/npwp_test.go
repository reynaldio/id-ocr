package npwp

import (
	"testing"

	"github.com/reynaldio/id-ocr/ocr"
)

const sampleNPWP = `KEMENTERIAN KEUANGAN REPUBLIK INDONESIA
DIREKTORAT JENDERAL PAJAK
NPWP : 09.254.294.3-407.000
NIK : 3171010905900001
Nama : BUDI SANTOSO
Alamat : JL MERDEKA NO 17
KPP : JAKARTA GAMBIR
Terdaftar : 21 Agustus 2008`

func TestParseNPWP(t *testing.T) {
	n, err := Parse(&ocr.Result{Text: sampleNPWP})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if n.Number != "09.254.294.3-407.000" {
		t.Errorf("Number = %q", n.Number)
	}
	if n.NIK != "3171010905900001" {
		t.Errorf("NIK = %q", n.NIK)
	}
	if n.Name != "BUDI SANTOSO" {
		t.Errorf("Name = %q", n.Name)
	}
	if n.KPP != "JAKARTA GAMBIR" {
		t.Errorf("KPP = %q", n.KPP)
	}
	if got := n.IssueDate.Format("2006-01-02"); got != "2008-08-21" {
		t.Errorf("IssueDate = %s, want 2008-08-21", got)
	}
	if err := n.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestFormat(t *testing.T) {
	if got := Format("092542943407000"); got != "09.254.294.3-407.000" {
		t.Errorf("Format = %q", got)
	}
	if got := Format("123"); got != "123" {
		t.Errorf("Format passthrough = %q", got)
	}
}

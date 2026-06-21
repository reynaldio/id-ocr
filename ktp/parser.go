package ktp

import (
	"regexp"
	"strings"
	"time"

	"github.com/reynaldio/id-ocr/idtype"
	"github.com/reynaldio/id-ocr/internal/normalize"
	"github.com/reynaldio/id-ocr/ocr"
)

var nikLineRe = regexp.MustCompile(`\d[\d\s]{14,}\d`)

// label maps a normalised line prefix to a setter on the KTP struct. KTP OCR
// is noisy, so matching is done on a lower-cased, despaced prefix.
type field struct {
	keys []string
	set  func(k *KTP, v string)
}

var fields = []field{
	{[]string{"nama"}, func(k *KTP, v string) { k.Name = v }},
	{[]string{"tempat/tgllahir", "tempattgllahir", "tempat/tgi", "tempattgi"}, func(k *KTP, v string) { setBirth(k, v) }},
	{[]string{"jeniskelamin"}, func(k *KTP, v string) { setSex(k, v) }},
	{[]string{"goldarah", "goldarah"}, func(k *KTP, v string) { k.BloodType = bloodOnly(v) }},
	{[]string{"alamat"}, func(k *KTP, v string) { k.Address = v }},
	{[]string{"rt/rw", "rtrw"}, func(k *KTP, v string) { k.RTRW = parseRTRW(v) }},
	{[]string{"kel/desa", "keldesa", "kelurahan"}, func(k *KTP, v string) { k.Village = v }},
	{[]string{"kecamatan"}, func(k *KTP, v string) { k.District = v }},
	{[]string{"agama"}, func(k *KTP, v string) { k.Religion = v }},
	{[]string{"statusperkawinan"}, func(k *KTP, v string) { k.MaritalStatus = v }},
	{[]string{"pekerjaan"}, func(k *KTP, v string) { k.Occupation = v }},
	{[]string{"kewarganegaraan"}, func(k *KTP, v string) { k.Nationality = v }},
	{[]string{"berlakuhingga"}, setValidUntil},
}

// addressKey is the matched label whose value continues onto unlabeled lines.
const addressKey = "alamat"

var rtRwRe = regexp.MustCompile(`(\d{1,3})\s*/\s*(\d{1,3})`)

// parseRTRW extracts the "NNN/NNN" RT/RW pattern, dropping OCR noise such as a
// leading "€" misread of the field's icon.
func parseRTRW(v string) string {
	if m := rtRwRe.FindStringSubmatch(v); m != nil {
		return m[1] + "/" + m[2]
	}
	return v
}

// setValidUntil canonicalises "Berlaku Hingga" into ValidUntilText (as printed)
// and ValidUntilDate (a time.Time). Lifetime cards read as "SEUMUR HIDUP" (OCR
// often truncates it or appends watermark fragments) and map ValidUntilDate to
// LifetimeDate; dated cards parse to the date.
func setValidUntil(k *KTP, v string) {
	u := strings.ToUpper(v)
	switch {
	case strings.Contains(u, "SEUMUR") || strings.Contains(u, "HIDUP"):
		k.ValidUntilText = "SEUMUR HIDUP"
		k.ValidUntilDate = LifetimeDate
	default:
		if d, err := normalize.ParseDate(v); err == nil {
			k.ValidUntilText = d.Format("02-01-2006")
			k.ValidUntilDate = d
		} else {
			k.ValidUntilText = u
		}
	}
}

// Parse builds a KTP from an OCR result. It is best-effort: missing or
// unreadable fields are left zero rather than failing. Call KTP.Validate to
// check the result.
func Parse(res *ocr.Result) (*KTP, error) {
	k := &KTP{RawText: res.Text}
	lines := res.Lines()

	inAddress := false // accumulating the multi-line Alamat value
	for _, raw := range lines {
		line := normalize.Spaces(raw)
		if k.scanHeader(line) || k.scanNIK(line) {
			inAddress = false
			continue
		}
		if mk, ok := k.scanLabel(line); ok {
			inAddress = mk == addressKey
			continue
		}
		// Unlabeled line: on a KTP the address wraps onto the line(s) below
		// "Alamat" before the next label, so append it there.
		if inAddress {
			if v := cleanValue(line); v != "" {
				k.Address = normalize.Spaces(k.Address + " " + v)
			}
		}
	}

	// Backfill sex from the NIK when OCR missed the label.
	if k.Sex == "" && k.NIK != "" {
		if s, ok := SexFromNIK(k.NIK); ok {
			k.Sex = s
		}
	}
	k.parseIssuing(lines)
	k.stripIssuingBleed()
	return k, nil
}

var trailingDateRe = regexp.MustCompile(`\s*\d{1,2}[-/.]\d{1,2}[-/.]\d{2,4}\s*$`)
var anyDateRe = regexp.MustCompile(`\d{1,2}[-/.]\d{1,2}[-/.]\d{2,4}`)

// parseIssuing fills IssuingCity (the Kabupaten/Kota of issue, which equals the
// Regency name) and IssuingDate (the date printed bottom-right). The issuing
// date is the first dd-mm-yyyy date in the OCR text that is neither the birth
// date nor the valid-until date.
func (k *KTP) parseIssuing(lines []string) {
	k.IssuingCity = strings.Join(regencyCityWords(k.Regency), " ")
	for _, line := range lines {
		for _, m := range anyDateRe.FindAllString(line, -1) {
			d, err := normalize.ParseDate(m)
			if err != nil {
				continue
			}
			if datesEqual(d, k.BirthDate) || datesEqual(d, k.ValidUntilDate) {
				continue
			}
			k.IssuingDate = d
			return
		}
	}
}

func datesEqual(a, b time.Time) bool {
	return !b.IsZero() && a.Equal(b)
}

// stripIssuingBleed removes text that bleeds in from the card's bottom-right
// issuing block — the city of issue (which equals the Regency name) and the
// issue date — which geometry reconstruction places on the same row as a value
// (e.g. "WNI BANDUNG BARAT", "KARYAWAN SWASTA 24-09-2020").
func (k *KTP) stripIssuingBleed() {
	city := regencyCityWords(k.Regency)
	k.Nationality = stripTrailingWords(k.Nationality, city)
	k.MaritalStatus = stripTrailingWords(k.MaritalStatus, city)
	k.Occupation = stripTrailingDate(stripTrailingWords(k.Occupation, city))
}

// regencyCityWords returns the city name words from a Regency, dropping the
// "KABUPATEN"/"KOTA" prefix (e.g. "KABUPATEN BANDUNG BARAT" -> [BANDUNG BARAT]).
func regencyCityWords(regency string) []string {
	w := strings.Fields(strings.ToUpper(regency))
	if len(w) > 0 && (w[0] == "KABUPATEN" || w[0] == "KOTA") {
		w = w[1:]
	}
	return w
}

func stripTrailingWords(s string, drop []string) string {
	if len(drop) == 0 {
		return s
	}
	set := make(map[string]bool, len(drop))
	for _, d := range drop {
		set[d] = true
	}
	words := strings.Fields(s)
	for len(words) > 1 && set[strings.ToUpper(words[len(words)-1])] {
		words = words[:len(words)-1]
	}
	return strings.Join(words, " ")
}

func stripTrailingDate(s string) string {
	return strings.TrimSpace(trailingDateRe.ReplaceAllString(s, ""))
}

// scanHeader captures the Province and Regency/City, which are printed as
// un-labelled all-caps headers identified by their leading keyword. Matching
// by keyword (rather than line position) is robust to OCR noise lines that
// often appear above the header. Returns true if consumed.
func (k *KTP) scanHeader(line string) bool {
	u := strings.ToUpper(line)
	switch {
	case k.Province == "" && strings.HasPrefix(u, "PROVINSI"):
		k.Province = normalize.Spaces(line[len("PROVINSI"):])
		return true
	case k.Regency == "" && (strings.HasPrefix(u, "KABUPATEN") || strings.HasPrefix(u, "KOTA")):
		k.Regency = normalize.Spaces(line)
		return true
	}
	return false
}

// scanNIK captures the 16-digit NIK. Returns true if consumed.
func (k *KTP) scanNIK(line string) bool {
	if k.NIK != "" {
		return false
	}
	if d := normalize.Digits(nikLineRe.FindString(line)); len(d) == 16 {
		k.NIK = d
		return true
	}
	return false
}

// scanLabel dispatches a "Label : value" line to its field setter and returns
// the matched label key. The value is taken after the colon when present,
// otherwise the matched label prefix is stripped — KTP OCR frequently drops the
// ":" separator.
func (k *KTP) scanLabel(line string) (string, bool) {
	key := despace(strings.ToLower(beforeColon(line)))
	for _, f := range fields {
		if mk, ok := matchKey(key, f.keys); ok {
			f.set(k, labelValue(line, mk))
			return mk, true
		}
	}
	return "", false
}

// labelValue extracts the value from a label line. With a colon it returns the
// text after it; otherwise it drops the first len(matchedKey) non-space
// characters (the label) from the front. Leading/trailing OCR punctuation
// noise (a misread ":" separator, stray "|", etc.) is trimmed.
func labelValue(line, matchedKey string) string {
	if i := strings.IndexByte(line, ':'); i >= 0 {
		return cleanValue(line[i+1:])
	}
	consumed := 0
	for idx, r := range line {
		if consumed >= len(matchedKey) {
			return cleanValue(line[idx:])
		}
		if r != ' ' {
			consumed++
		}
	}
	return ""
}

// edgeNoise is punctuation that commonly bleeds onto KTP field values from a
// misread separator or border, but never legitimately starts or ends one.
const edgeNoise = " .,:;|/\\-_'\"`“”‘’*"

func cleanValue(s string) string {
	return strings.Trim(normalize.Spaces(s), edgeNoise)
}

func setBirth(k *KTP, v string) {
	// "JAKARTA, 17-08-1990" -> place + date
	place := v
	if i := strings.LastIndex(v, ","); i >= 0 {
		place = normalize.Spaces(v[:i])
	}
	k.BirthPlace = strings.ToUpper(place)
	if d, err := normalize.ParseDate(v); err == nil {
		k.BirthDate = d
	}
}

func setSex(k *KTP, v string) {
	u := strings.ToUpper(v)
	switch {
	case strings.Contains(u, "PEREMPUAN"):
		k.Sex = idtype.Female
	case strings.Contains(u, "LAKI"):
		k.Sex = idtype.Male
	}
	// On a real KTP the "Gol. Darah" field shares the line with "Jenis
	// Kelamin"; capture it here if present.
	if i := strings.Index(u, "DARAH"); i >= 0 && k.BloodType == "" {
		k.BloodType = bloodOnly(u[i+len("DARAH"):])
	}
}

func bloodOnly(v string) string {
	// Jenis Kelamin line often carries "Gol. Darah: O"; keep just the group.
	u := strings.ToUpper(v)
	for _, g := range []string{"AB", "A", "B", "O"} {
		if strings.Contains(u, g) {
			return g
		}
	}
	return "-"
}

func beforeColon(s string) string {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return s[:i]
	}
	return s
}

func despace(s string) string { return strings.ReplaceAll(s, " ", "") }

// matchKey reports whether key starts with any of keys, returning the matched
// entry so the caller can strip exactly that label from the line.
func matchKey(key string, keys []string) (string, bool) {
	for _, k := range keys {
		if strings.HasPrefix(key, k) {
			return k, true
		}
	}
	return "", false
}

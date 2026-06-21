// Package idtype holds normalized value types shared across the document
// parsers, so a common field has the same representation on every document
// (e.g. Sex is "MALE"/"FEMALE" whether it came from a KTP or a passport MRZ).
package idtype

// Sex is a person's recorded sex, normalized to a single representation across
// all document types. The zero value "" means unknown/unreadable.
type Sex string

const (
	Male        Sex = "MALE"
	Female      Sex = "FEMALE"
	Unspecified Sex = "UNSPECIFIED" // recorded as "X" on some passports
)

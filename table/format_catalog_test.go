package table

import "testing"

func TestValueFormatNumericRanges(t *testing.T) {
	vf := &ValueFormat{Name: "agegrp", Ranges: []FormatRange{
		{NoLow: true, High: Num(12), Label: "Child"},
		{Low: Num(13), High: Num(19), Label: "Teen"},
		{Low: Num(20), NoHigh: true, Label: "Adult"},
	}}
	cases := []struct {
		in   float64
		want string
	}{
		{5, "Child"}, {12, "Child"}, {13, "Teen"}, {19, "Teen"}, {20, "Adult"}, {99, "Adult"},
	}
	for _, c := range cases {
		got, ok := vf.Format(Num(c.in))
		if !ok || got != c.want {
			t.Errorf("Format(%v) = %q,%v; want %q", c.in, got, ok, c.want)
		}
	}
	// A value below the lowest bound with no `other` is unmatched.
	if _, ok := vf.Format(MissingNum()); ok {
		t.Error("missing value should not match a numeric range")
	}
}

func TestValueFormatExclusiveBounds(t *testing.T) {
	// 0 <- 10 (exclusive low, inclusive high); 10 -< 20 (incl low, excl high).
	vf := &ValueFormat{Name: "g", Ranges: []FormatRange{
		{Low: Num(0), High: Num(10), LowExcl: true, Label: "A"},
		{Low: Num(10), High: Num(20), HighExcl: true, Label: "B"},
	}}
	if l, _ := vf.Format(Num(10)); l != "A" {
		t.Errorf("10 -> %q, want A (10 is the inclusive high of A)", l)
	}
	if l, _ := vf.Format(Num(15)); l != "B" {
		t.Errorf("15 -> %q, want B", l)
	}
	if _, ok := vf.Format(Num(0)); ok {
		t.Error("0 should be excluded (exclusive low of A)")
	}
	if _, ok := vf.Format(Num(20)); ok {
		t.Error("20 should be excluded (exclusive high of B)")
	}
}

func TestValueFormatOtherAndChar(t *testing.T) {
	vf := &ValueFormat{Name: "$sex", Char: true, Ranges: []FormatRange{
		{Low: Char("M"), Label: "Male"},
		{Low: Char("F"), Label: "Female"},
		{Other: true, Label: "Unknown"},
	}}
	if l, _ := vf.Format(Char("M")); l != "Male" {
		t.Errorf(`"M" -> %q, want Male`, l)
	}
	if l, _ := vf.Format(Char("X")); l != "Unknown" {
		t.Errorf(`"X" -> %q, want Unknown (other)`, l)
	}
}

func TestFormatCatalogDefineLookup(t *testing.T) {
	cat := NewFormatCatalog()
	cat.Define(&ValueFormat{Name: "AgeGrp"})
	if _, ok := cat.Lookup("agegrp"); !ok {
		t.Error("Lookup should be case-insensitive")
	}
	if _, ok := cat.Lookup("nope"); ok {
		t.Error("Lookup of an undefined format should fail")
	}
	var nilCat *FormatCatalog
	if _, ok := nilCat.Lookup("x"); ok {
		t.Error("nil catalog Lookup should be safe and return false")
	}
}

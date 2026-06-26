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

func TestPictureFormat(t *testing.T) {
	// Currency: comma grouping, two decimals (default mult 100), $ prefix.
	dollars := &ValueFormat{Name: "dollars", Picture: true, Ranges: []FormatRange{
		{NoLow: true, NoHigh: true, Label: "000,000,009.99", Prefix: "$"},
	}}
	cases := []struct {
		in   float64
		want string
	}{
		{1234.5, "      $1,234.50"},
		{58.07, "         $58.07"},
		{0, "          $0.00"},
	}
	for _, c := range cases {
		if got, ok := dollars.Format(Num(c.in)); !ok || got != c.want {
			t.Errorf("dollars.Format(%v) = %q,%v; want %q", c.in, got, ok, c.want)
		}
	}

	// Phone: all zero-suppress selectors with a literal dash; no leading zeros lost
	// because the value fills every selector.
	phone := &ValueFormat{Name: "phone", Picture: true, Ranges: []FormatRange{
		{Other: true, Label: "000-0000"},
	}}
	if got, _ := phone.Format(Num(5551234)); got != "555-1234" {
		t.Errorf("phone.Format(5551234) = %q; want 555-1234", got)
	}

	// Percent: default mult from one decimal selector; a trailing message char.
	pct := &ValueFormat{Name: "pct", Picture: true, Ranges: []FormatRange{
		{NoLow: true, NoHigh: true, Label: "009.9%"},
	}}
	for _, c := range []struct {
		in   float64
		want string
	}{{7.3, "  7.3%"}, {100, "100.0%"}, {0.5, "  0.5%"}} {
		if got, _ := pct.Format(Num(c.in)); got != c.want {
			t.Errorf("pct.Format(%v) = %q; want %q", c.in, got, c.want)
		}
	}

	// Explicit mult and fill: scale by 1 (integer), zero-fill leading positions.
	zfill := &ValueFormat{Name: "z", Picture: true, Ranges: []FormatRange{
		{NoLow: true, NoHigh: true, Label: "000000", Mult: 1, Fill: '0'},
	}}
	if got, _ := zfill.Format(Num(42)); got != "000042" {
		t.Errorf("zfill.Format(42) = %q; want 000042", got)
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

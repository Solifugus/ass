package formats

import (
	"testing"

	"github.com/solifugus/ass/table"
)

func TestApplyNumeric(t *testing.T) {
	cases := []struct {
		val    float64
		format string
		want   string
	}{
		{1234.5, "8.2", "1234.50"},
		{1234.567, "8.1", "1234.6"},
		{1234567, "comma12.", "1,234,567"},
		{1234.5, "comma10.2", "1,234.50"},
		{1234.5, "dollar12.2", "$1,234.50"},
		{-1234.5, "dollar12.2", "-$1,234.50"},
		{0.25, "percent8.1", "25.0%"},
		{42, "best.", "42"},
	}
	for _, c := range cases {
		if got := Apply(table.Num(c.val), c.format); got != c.want {
			t.Errorf("Apply(%v, %q) = %q, want %q", c.val, c.format, got, c.want)
		}
	}
}

func TestDateLiteralAndFormats(t *testing.T) {
	if d, ok := ParseDateLiteral("01JAN1960"); !ok || d != 0 {
		t.Errorf("01JAN1960 = %v ok=%v, want 0", d, ok)
	}
	day, ok := ParseDateLiteral("15MAR2021")
	if !ok {
		t.Fatal("failed to parse 15MAR2021")
	}
	cases := []struct {
		format string
		want   string
	}{
		{"date9.", "15MAR2021"},
		{"date7.", "15MAR21"},
		{"mmddyy10.", "03/15/2021"},
		{"mmddyy8.", "03/15/21"},
		{"worddate.", "March 15, 2021"},
	}
	for _, c := range cases {
		// Strip the trailing '.' as the parser does before calling Apply.
		spec := c.format
		if spec[len(spec)-1] == '.' {
			spec = spec[:len(spec)-1]
		}
		if got := Apply(table.Num(day), spec); got != c.want {
			t.Errorf("Apply(15MAR2021, %q) = %q, want %q", c.format, got, c.want)
		}
	}
}

func TestApplyMissingAndChar(t *testing.T) {
	if got := Apply(table.MissingNum(), "dollar8.2"); got != "." {
		t.Errorf("missing = %q, want .", got)
	}
	if got := Apply(table.Char("hello world"), "$5."); got != "hello" {
		t.Errorf("char $5. = %q, want hello", got)
	}
	if got := Apply(table.Num(5), ""); got != "5" {
		t.Errorf("empty format = %q, want 5", got)
	}
}

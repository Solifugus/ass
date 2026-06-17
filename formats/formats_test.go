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

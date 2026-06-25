package formats

import (
	"testing"

	"github.com/solifugus/ass/table"
)

// TestBestNumDefault checks SAS BEST12.-style default numeric rendering: bounded
// width, trailing zeros trimmed, integers without a decimal point, and no Go
// shortest-exact float noise.
func TestBestNumDefault(t *testing.T) {
	cases := []struct {
		val  float64
		want string
	}{
		{47.74934554525329, "47.749345545"}, // 12 cols, was full precision
		{539.2599095056112, "539.25990951"}, // 12 cols
		{925.5, "925.5"},                    // trailing zeros trimmed
		{129, "129"},                        // integer, no decimal point
		{0, "0"},
		{-12.5, "-12.5"},
		{4.5, "4.5"},
		{1234567, "1234567"},                 // 7-digit integer fits
		{0.3333333333333333, "0.3333333333"}, // sub-1 value fills width (0. + 10 digits)
		{-0.5, "-0.5"},
	}
	for _, c := range cases {
		if got := Apply(table.Num(c.val), ""); got != c.want {
			t.Errorf("Apply(%v, \"\") = %q, want %q", c.val, got, c.want)
		}
	}
}

// TestBestNumWidth honors an explicit BESTw. width.
func TestBestNumWidth(t *testing.T) {
	if got := Apply(table.Num(47.74934554525329), "best8."); got != "47.74935" {
		t.Errorf("best8. = %q, want %q", got, "47.74935")
	}
}

// TestBestNumMissing renders the missing value as a dot.
func TestBestNumMissing(t *testing.T) {
	if got := Apply(table.MissingNum(), ""); got != "." {
		t.Errorf("missing default = %q, want .", got)
	}
}

// TestApplyEmptyCharacter confirms the empty-format path leaves characters as-is.
func TestApplyEmptyCharacter(t *testing.T) {
	if got := Apply(table.Char("hello"), ""); got != "hello" {
		t.Errorf("Apply(char, \"\") = %q, want hello", got)
	}
}

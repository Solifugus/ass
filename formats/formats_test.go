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

func TestParseInputNumericAndChar(t *testing.T) {
	cases := []struct {
		field, informat string
		wantNum         float64
		wantStr         string
		missing         bool
	}{
		{"1,234", "comma8.", 1234, "", false},
		{"$56,789.50", "dollar12.2", 56789.5, "", false},
		{"42", "8.", 42, "", false},
		{"abc", "8.", 0, "", true},
		{".", "comma8.", 0, "", true},
		{"Hello World", "$5.", 0, "Hello", false}, // truncated to width 5
	}
	for _, c := range cases {
		got := ParseInput(c.field, c.informat)
		if c.missing {
			if !got.IsMissing() {
				t.Errorf("ParseInput(%q,%q) = %v, want missing", c.field, c.informat, got.Display())
			}
			continue
		}
		if c.wantStr != "" {
			if got.Str != c.wantStr {
				t.Errorf("ParseInput(%q,%q) = %q, want %q", c.field, c.informat, got.Str, c.wantStr)
			}
		} else if got.Num != c.wantNum {
			t.Errorf("ParseInput(%q,%q) = %v, want %v", c.field, c.informat, got.Num, c.wantNum)
		}
	}
}

func TestParseInputDates(t *testing.T) {
	// date9. and mmddyy. should agree on the same calendar date's SAS day number.
	d9 := ParseInput("15JAN2020", "date9.")
	mdy := ParseInput("01/15/2020", "mmddyy10.")
	if d9.IsMissing() || mdy.IsMissing() {
		t.Fatalf("date parse failed: date9=%v mmddyy=%v", d9.Display(), mdy.Display())
	}
	if d9.Num != mdy.Num {
		t.Errorf("date9 %v != mmddyy %v for 2020-01-15", d9.Num, mdy.Num)
	}
	// ddmmyy reads day first.
	dmy := ParseInput("15/01/2020", "ddmmyy10.")
	if dmy.Num != d9.Num {
		t.Errorf("ddmmyy %v != date9 %v", dmy.Num, d9.Num)
	}
	// yymmdd packed.
	ymd := ParseInput("20200115", "yymmdd8.")
	if ymd.Num != d9.Num {
		t.Errorf("yymmdd %v != date9 %v", ymd.Num, d9.Num)
	}
}

func TestTimeLiteralAndFormats(t *testing.T) {
	// '14:30:00't -> 14*3600 + 30*60 = 52200 seconds.
	sec, ok := ParseTimeLiteral("14:30:00")
	if !ok || sec != 52200 {
		t.Fatalf("ParseTimeLiteral = %v,%v want 52200", sec, ok)
	}
	// HH:MM form.
	if s, ok := ParseTimeLiteral("9:15"); !ok || s != 9*3600+15*60 {
		t.Errorf("ParseTimeLiteral(9:15) = %v,%v", s, ok)
	}
	// Round-trip through the time informat and time format.
	v := ParseInput("14:30:00", "time8.")
	if v.IsMissing() || v.Num != 52200 {
		t.Fatalf("ParseInput time8. = %v", v.Display())
	}
	if got := Apply(v, "time8."); got != "14:30:00" {
		t.Errorf("Apply time8. = %q, want 14:30:00", got)
	}
	if got := Apply(v, "time5."); got != "14:30" {
		t.Errorf("Apply time5. = %q, want 14:30", got)
	}
}

func TestDatetimeLiteralAndFormats(t *testing.T) {
	// Datetime = date-day * 86400 + time-seconds.
	day, _ := ParseDateLiteral("01JAN2020")
	dt, ok := ParseDatetimeLiteral("01JAN2020:14:30:00")
	if !ok || dt != day*86400+52200 {
		t.Fatalf("ParseDatetimeLiteral = %v,%v want %v", dt, ok, day*86400+52200)
	}
	// Round-trip through the datetime informat and format.
	v := ParseInput("01JAN2020:14:30:00", "datetime19.")
	if v.IsMissing() || v.Num != dt {
		t.Fatalf("ParseInput datetime19. = %v", v.Display())
	}
	if got := Apply(v, "datetime19."); got != "01JAN2020:14:30:00" {
		t.Errorf("Apply datetime19. = %q, want 01JAN2020:14:30:00", got)
	}
}

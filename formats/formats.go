package formats

import (
	"strconv"
	"strings"

	"github.com/solifugus/ass/table"
)

// Apply renders a value using a SAS format specification (e.g. "8.2",
// "dollar10.2", "comma12.0", "percent8.1", "$8."). An empty or unrecognized
// format falls back to the value's default display. Numeric missing renders as
// ".".
func Apply(v table.Value, format string) string {
	if format == "" {
		return v.Display()
	}
	name, width, dec, hasDec := parseSpec(format)

	if v.Kind == table.Character || name == "$" {
		s := v.Str
		if width > 0 && len(s) > width {
			s = s[:width]
		}
		return s
	}
	if v.IsMissing() {
		return "."
	}

	switch name {
	case "dollar":
		n, sign := v.Num, ""
		if n < 0 {
			n, sign = -n, "-"
		}
		return sign + "$" + group(n, decOr(dec, hasDec, 2))
	case "comma":
		return group(v.Num, decOr(dec, hasDec, 0))
	case "percent":
		return fixed(v.Num*100, decOr(dec, hasDec, 0)) + "%"
	default: // "", "best", "f" → fixed-point if decimals given, else default
		if hasDec {
			return fixed(v.Num, dec)
		}
		return v.Display()
	}
}

// decOr returns dec if a decimal count was specified, otherwise def.
func decOr(dec int, hasDec bool, def int) int {
	if hasDec {
		return dec
	}
	return def
}

// fixed formats n with exactly d decimal places.
func fixed(n float64, d int) string {
	return strconv.FormatFloat(n, 'f', d, 64)
}

// group formats n with thousands separators and d decimal places.
func group(n float64, d int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatFloat(n, 'f', d, 64)
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	// Insert commas every three digits from the right.
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String() + frac
	if neg {
		out = "-" + out
	}
	return out
}

// parseSpec splits a format specification into its name, width, and optional
// decimal count. Examples: "8.2" -> ("",8,2,true); "dollar10.2" ->
// ("dollar",10,2,true); "$8." -> ("$",8,0,false); "comma12." -> ("comma",12,0,false).
func parseSpec(f string) (name string, width, dec int, hasDec bool) {
	i := 0
	if i < len(f) && f[i] == '$' {
		name = "$"
		i++
	}
	for i < len(f) && isLetter(f[i]) {
		i++
	}
	if name == "" {
		name = strings.ToLower(f[:i])
	} else {
		name += strings.ToLower(f[1:i])
	}
	// width digits
	j := i
	for j < len(f) && isDigit(f[j]) {
		j++
	}
	width, _ = strconv.Atoi(f[i:j])
	if j < len(f) && f[j] == '.' {
		k := j + 1
		for k < len(f) && isDigit(f[k]) {
			k++
		}
		if k > j+1 {
			dec, _ = strconv.Atoi(f[j+1 : k])
			hasDec = true
		}
	}
	return name, width, dec, hasDec
}

func isLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
func isDigit(b byte) bool  { return b >= '0' && b <= '9' }

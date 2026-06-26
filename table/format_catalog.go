package table

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// FormatRange is one mapping in a user-defined VALUE format: input values that
// fall in the range render as Label. A range is either a single value, a
// low-high interval (bounds optionally exclusive or open-ended), or the
// catch-all "other".
type FormatRange struct {
	Low      Value // lower bound (also the single value when High == Low)
	High     Value // upper bound
	NoLow    bool  // unbounded below (the `low` keyword)
	NoHigh   bool  // unbounded above (the `high` keyword)
	LowExcl  bool  // lower bound is exclusive (`a <- b`)
	HighExcl bool  // upper bound is exclusive (`a -< b`)
	Other    bool  // catch-all `other=`
	Label    string

	// PICTURE per-range options. Label holds the digit-selector template; these
	// refine its rendering. Mult==0 means "use the template's default multiplier".
	Prefix string
	Mult   float64
	Fill   byte
}

// ValueFormat is a user-defined format created by PROC FORMAT's VALUE statement.
// Char is true for a character format (its name began with `$`).
type ValueFormat struct {
	Name    string
	Char    bool
	Picture bool // a PICTURE format: matched labels are digit-selector templates
	Ranges  []FormatRange
}

// Format returns the label for v if it matches one of the format's ranges. The
// second result is false when nothing matched, so the caller can fall back to a
// default rendering. Ranges are tested in definition order; an `other` range
// matches anything not matched earlier.
func (vf *ValueFormat) Format(v Value) (string, bool) {
	var other *FormatRange
	for i := range vf.Ranges {
		r := &vf.Ranges[i]
		if r.Other {
			if other == nil {
				other = r
			}
			continue
		}
		if r.matches(v, vf.Char) {
			return vf.render(r, v), true
		}
	}
	if other != nil {
		return vf.render(other, v), true
	}
	return "", false
}

// render produces the output text for value v matched by range r. For a PICTURE
// format the matched label is a digit-selector template; otherwise the label is
// returned verbatim.
func (vf *ValueFormat) render(r *FormatRange, v Value) string {
	if vf.Picture && !vf.Char {
		if v.IsMissing() {
			return " "
		}
		return r.renderPicture(v.Num)
	}
	return r.Label
}

// matches reports whether v falls within range r.
func (r *FormatRange) matches(v Value, char bool) bool {
	if char {
		// Character ranges are exact matches (comma lists are expanded into one
		// single-value range each at definition time).
		return v.Kind == Character && v.Str == r.Low.Str
	}
	if v.IsMissing() {
		return false
	}
	if !r.NoLow {
		c := v.Compare(r.Low)
		if c < 0 || (c == 0 && r.LowExcl) {
			return false
		}
	}
	if !r.NoHigh {
		c := v.Compare(r.High)
		if c > 0 || (c == 0 && r.HighExcl) {
			return false
		}
	}
	return true
}

// renderPicture renders a non-negative-magnitude numeric value through this
// range's PICTURE template (held in Label). Digit selectors (0-9) are positions
// the value's digits fill right to left; a selector of 0 zero-suppresses leading
// positions (printed as the fill char, blank by default) while a nonzero selector
// forces printing from its position rightward. Non-selector characters are
// message characters, printed only once significant digits have begun. The value
// is scaled by Mult (default: 10^(selectors after the decimal point)) and rounded
// to an integer before mapping. Prefix is inserted just before the first printed
// digit. The sign is dropped, matching SAS picture behavior.
func (r *FormatRange) renderPicture(num float64) string {
	tmpl := r.Label
	// Locate digit-selector positions and count those after a decimal point.
	var sel []int
	decimals, seenDot := 0, false
	for i := 0; i < len(tmpl); i++ {
		c := tmpl[i]
		switch {
		case c >= '0' && c <= '9':
			sel = append(sel, i)
			if seenDot {
				decimals++
			}
		case c == '.':
			seenDot = true
		}
	}
	d := len(sel)
	if d == 0 {
		return tmpl // no digit selectors: a constant picture
	}

	mult := r.Mult
	if mult == 0 {
		mult = math.Pow(10, float64(decimals))
	}
	scaled := int64(math.Round(math.Abs(num) * mult))
	digits := strconv.FormatInt(scaled, 10)
	switch {
	case len(digits) < d:
		digits = strings.Repeat("0", d-len(digits)) + digits
	case len(digits) > d:
		digits = digits[len(digits)-d:] // overflow: keep the low-order d digits
	}

	// firstSig: index of the first significant (nonzero) digit in the padded value.
	firstSig := d
	for k := 0; k < d; k++ {
		if digits[k] != '0' {
			firstSig = k
			break
		}
	}
	// forceFrom: index of the first nonzero digit selector (forces printing).
	forceFrom := d
	for k := 0; k < d; k++ {
		if tmpl[sel[k]] != '0' {
			forceFrom = k
			break
		}
	}
	startPrint := firstSig
	if forceFrom < startPrint {
		startPrint = forceFrom
	}

	fill := byte(' ')
	if r.Fill != 0 {
		fill = r.Fill
	}

	var b strings.Builder
	selIdx, started, prefixed := 0, false, false
	for i := 0; i < len(tmpl); i++ {
		c := tmpl[i]
		if c >= '0' && c <= '9' {
			k := selIdx
			selIdx++
			if k >= startPrint {
				if !prefixed {
					b.WriteString(r.Prefix)
					prefixed = true
				}
				b.WriteByte(digits[k])
				started = true
			} else {
				b.WriteByte(fill)
			}
			continue
		}
		// Message character: shown once printing has begun, else blanked.
		if started {
			b.WriteByte(c)
		} else {
			b.WriteByte(fill)
		}
	}
	return b.String()
}

// FormatCatalog holds the user-defined formats for one run, keyed by lowercased
// name (character formats keep their leading `$`). It models the per-program
// scope of PROC FORMAT definitions; a fresh Library starts with an empty one so
// definitions never leak between programs (e.g. across the test harness).
type FormatCatalog struct {
	formats map[string]*ValueFormat
}

// NewFormatCatalog returns an empty catalog.
func NewFormatCatalog() *FormatCatalog {
	return &FormatCatalog{formats: map[string]*ValueFormat{}}
}

// Define stores (or replaces) a format under its name.
func (c *FormatCatalog) Define(vf *ValueFormat) {
	c.formats[strings.ToLower(vf.Name)] = vf
}

// Lookup returns the format registered under name, if any. It is nil-safe so
// callers can pass a possibly-absent catalog.
func (c *FormatCatalog) Lookup(name string) (*ValueFormat, bool) {
	if c == nil {
		return nil, false
	}
	vf, ok := c.formats[strings.ToLower(name)]
	return vf, ok
}

// All returns the catalog's formats sorted by name, for deterministic output
// (e.g. PROC FORMAT CNTLOUT=). Nil-safe.
func (c *FormatCatalog) All() []*ValueFormat {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.formats))
	for k := range c.formats {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]*ValueFormat, len(names))
	for i, k := range names {
		out[i] = c.formats[k]
	}
	return out
}

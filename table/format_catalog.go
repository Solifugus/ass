package table

import "strings"

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
}

// ValueFormat is a user-defined format created by PROC FORMAT's VALUE statement.
// Char is true for a character format (its name began with `$`).
type ValueFormat struct {
	Name   string
	Char   bool
	Ranges []FormatRange
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
			return r.Label, true
		}
	}
	if other != nil {
		return other.Label, true
	}
	return "", false
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

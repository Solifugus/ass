package table

import (
	"strconv"
	"strings"
)

// InformatRange is one mapping in a user-defined INVALUE informat: an input that
// matches the range converts to Result. A range is either a string key (exact
// match against the trimmed input), a numeric interval (the input parsed as a
// number and tested against [Low,High], bounds optionally exclusive/open), or the
// catch-all `other`.
type InformatRange struct {
	Other    bool
	Numeric  bool // match the input parsed as a number against [Low,High]
	Key      string
	Low      float64
	High     float64
	NoLow    bool
	NoHigh   bool
	LowExcl  bool
	HighExcl bool
	Result   Value
}

// UserInformat is a user-defined informat created by PROC FORMAT's INVALUE
// statement. Char is true when the result is character (the name began with `$`).
type UserInformat struct {
	Name   string
	Char   bool
	Ranges []InformatRange
}

// Parse converts an input field through the informat, returning the mapped value
// and whether anything matched (so the caller can fall back). Ranges are tested
// in order; an `other` range matches anything not matched earlier.
func (inf *UserInformat) Parse(field string) (Value, bool) {
	f := strings.TrimSpace(field)
	var other *InformatRange
	for i := range inf.Ranges {
		r := &inf.Ranges[i]
		if r.Other {
			if other == nil {
				other = r
			}
			continue
		}
		if r.Numeric {
			if x, err := strconv.ParseFloat(f, 64); err == nil && r.matchesNum(x) {
				return r.Result, true
			}
			continue
		}
		if f == r.Key {
			return r.Result, true
		}
	}
	if other != nil {
		return other.Result, true
	}
	return Value{}, false
}

func (r *InformatRange) matchesNum(x float64) bool {
	if !r.NoLow {
		if x < r.Low || (x == r.Low && r.LowExcl) {
			return false
		}
	}
	if !r.NoHigh {
		if x > r.High || (x == r.High && r.HighExcl) {
			return false
		}
	}
	return true
}

// InformatCatalog holds the user-defined informats for one run, keyed by
// lowercased name (character informats keep their leading `$`).
type InformatCatalog struct {
	informats map[string]*UserInformat
}

// NewInformatCatalog returns an empty catalog.
func NewInformatCatalog() *InformatCatalog {
	return &InformatCatalog{informats: map[string]*UserInformat{}}
}

// Define stores (or replaces) an informat under its name.
func (c *InformatCatalog) Define(inf *UserInformat) {
	c.informats[strings.ToLower(inf.Name)] = inf
}

// Lookup returns the informat registered under name, if any. Nil-safe.
func (c *InformatCatalog) Lookup(name string) (*UserInformat, bool) {
	if c == nil {
		return nil, false
	}
	inf, ok := c.informats[strings.ToLower(name)]
	return inf, ok
}

// Package table provides the dataset abstraction: datasets, columns with
// types/labels/formats, rows, values with missing-value semantics, and the
// in-memory library that steps use to pass data.
package table

import (
	"strconv"
	"strings"
)

// Kind is the storage type of a SAS variable/value: numeric or character.
type Kind int

const (
	Numeric Kind = iota
	Character
)

func (k Kind) String() string {
	if k == Character {
		return "char"
	}
	return "num"
}

// Value is a single SAS data value. SAS has exactly two types: numeric
// (float64) and character (string). Missing values follow SAS rules: a numeric
// missing is a distinct state (displayed "."), while a character missing is
// simply the empty string.
type Value struct {
	Kind    Kind
	Num     float64
	Str     string
	missing bool // only meaningful for Numeric
}

// Num returns a numeric value.
func Num(f float64) Value { return Value{Kind: Numeric, Num: f} }

// Char returns a character value.
func Char(s string) Value { return Value{Kind: Character, Str: s} }

// MissingNum returns the numeric missing value (.).
func MissingNum() Value { return Value{Kind: Numeric, missing: true} }

// MissingChar returns the character missing value (empty string).
func MissingChar() Value { return Value{Kind: Character, Str: ""} }

// IsMissing reports whether the value is missing. Numeric uses the missing
// flag; character is missing when it is the empty string.
func (v Value) IsMissing() bool {
	if v.Kind == Character {
		return v.Str == ""
	}
	return v.missing
}

// Compare orders two values by SAS rules and returns -1, 0, or 1. Character
// values (when either side is character) compare lexically; numeric values
// compare by magnitude with missing ordered below every non-missing number (two
// missings are equal). This is the canonical ordering used by comparisons and by
// PROC SORT.
func (v Value) Compare(o Value) int {
	if v.Kind == Character || o.Kind == Character {
		return strings.Compare(v.Str, o.Str)
	}
	vm, om := v.IsMissing(), o.IsMissing()
	switch {
	case vm && om:
		return 0
	case vm:
		return -1
	case om:
		return 1
	case v.Num < o.Num:
		return -1
	case v.Num > o.Num:
		return 1
	default:
		return 0
	}
}

// Display renders the value the way it appears unformatted: "." for numeric
// missing, the string as-is for character, and a compact decimal for numbers.
func (v Value) Display() string {
	if v.Kind == Character {
		return v.Str
	}
	if v.missing {
		return "."
	}
	return strconv.FormatFloat(v.Num, 'g', -1, 64)
}

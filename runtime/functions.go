package runtime

import (
	"fmt"
	"math"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/formats"
	"github.com/solifugus/ass/table"
)

// applyPutFormat renders v through a format spec for the PUT() function: a user
// VALUE format from the catalog if one matches, otherwise the built-in formats.
func applyPutFormat(cat *table.FormatCatalog, v table.Value, spec string) string {
	if cat != nil && spec != "" {
		key := strings.TrimSuffix(strings.ToLower(spec), ".")
		if vf, ok := lookupUserFormatRT(cat, key); ok {
			if label, matched := vf.Format(v); matched {
				return label
			}
			return v.Display()
		}
	}
	return formats.Apply(v, spec)
}

// lookupUserFormatRT finds a user format by name, retrying with trailing
// display-width digits stripped (e.g. "agegrp8" -> "agegrp").
func lookupUserFormatRT(cat *table.FormatCatalog, key string) (*table.ValueFormat, bool) {
	if vf, ok := cat.Lookup(key); ok {
		return vf, true
	}
	if bare := strings.TrimRight(key, "0123456789"); bare != key && bare != "" && bare != "$" {
		return cat.Lookup(bare)
	}
	return nil, false
}

// applyInputInformat reads field through an informat spec for the INPUT()
// function: a user INVALUE informat from the catalog if one matches, otherwise
// the built-in informats.
func applyInputInformat(cat *table.InformatCatalog, field, spec string) table.Value {
	name := strings.TrimSuffix(strings.ToLower(spec), ".")
	if cat != nil {
		try := func(n string) (table.Value, bool) {
			inf, ok := cat.Lookup(n)
			if !ok {
				return table.Value{}, false
			}
			if val, matched := inf.Parse(field); matched {
				return val, true
			}
			if inf.Char {
				return table.MissingChar(), true
			}
			return table.MissingNum(), true
		}
		if val, ok := try(name); ok {
			return val
		}
		if bare := strings.TrimRight(name, "0123456789"); bare != name {
			if val, ok := try(bare); ok {
				return val
			}
		}
	}
	return formats.ParseInput(field, spec)
}

// evalCall evaluates a function call. Functions follow SAS conventions: the
// aggregate functions (sum, mean, min, max, n) ignore missing arguments, while
// most scalar functions propagate missing.
func evalCall(e *ast.CallExpression, pdv *PDV) (table.Value, error) {
	args := make([]table.Value, len(e.Args))
	for i, a := range e.Args {
		v, err := Eval(a, pdv)
		if err != nil {
			return table.MissingNum(), err
		}
		args[i] = v
	}

	switch strings.ToLower(e.Func) {
	case "sum":
		return aggregate(args, 0, func(acc, x float64) float64 { return acc + x }), nil
	case "mean", "avg":
		return meanFn(args), nil
	case "min":
		return reduce(args, func(acc, x float64) float64 { return math.Min(acc, x) }), nil
	case "max":
		return reduce(args, func(acc, x float64) float64 { return math.Max(acc, x) }), nil
	case "n":
		return table.Num(float64(countNonMissing(args))), nil
	case "nmiss":
		return table.Num(float64(len(args) - countNonMissing(args))), nil
	case "abs":
		return scalar1(args, math.Abs)
	case "int":
		return scalar1(args, math.Trunc)
	case "ceil":
		return scalar1(args, math.Ceil)
	case "floor":
		return scalar1(args, math.Floor)
	case "sqrt":
		return scalar1(args, math.Sqrt)
	case "exp":
		return scalar1(args, math.Exp)
	case "log":
		return scalar1(args, math.Log)
	case "round":
		return roundFn(args)
	case "upcase":
		return scalarStr(args, strings.ToUpper)
	case "lowcase":
		return scalarStr(args, strings.ToLower)
	case "trim":
		return scalarStr(args, func(s string) string { return strings.TrimRight(s, " ") })
	case "strip":
		return scalarStr(args, strings.TrimSpace)
	case "left":
		return scalarStr(args, func(s string) string { return strings.TrimLeft(s, " ") })
	case "length":
		return lengthFn(args), nil
	case "substr":
		return substrFn(args)
	case "cats":
		return catsFn(args), nil
	case "catx":
		return catxFn(args)
	case "index":
		return indexFn(args, 0)
	case "find":
		return findFn(args)
	case "scan":
		return scanFn(args)
	case "compress":
		return compressFn(args)
	case "tranwrd":
		return tranwrdFn(args)
	case "propcase":
		return scalarStr(args, propcase)
	case "reverse":
		return scalarStr(args, reverse)
	case "missing":
		if len(args) != 1 {
			return table.MissingNum(), fmt.Errorf("missing expects 1 argument, got %d", len(args))
		}
		return boolVal(args[0].IsMissing()), nil
	case "today", "date":
		return todayFn(args)
	case "datetime":
		return datetimeFn(args)
	case "time":
		return timeFn(args)
	case "mdy":
		return mdyFn(args)
	case "year":
		return yearFn(args)
	case "month":
		return monthFn(args)
	case "day":
		return dayFn(args)
	case "qtr":
		return qtrFn(args)
	case "weekday":
		return weekdayFn(args)
	case "datepart":
		return datepartFn(args)
	case "timepart":
		return timepartFn(args)
	case "hms":
		return hmsFn(args)
	case "dhms":
		return dhmsFn(args)
	case "intck":
		return intckFn(args)
	case "intnx":
		return intnxFn(args)
	case "put":
		// put(value, format.) -> the value rendered through the format, as a
		// character string (user VALUE formats and built-in formats both apply).
		if len(args) < 2 {
			return table.MissingChar(), fmt.Errorf("put expects 2 arguments, got %d", len(args))
		}
		return table.Char(applyPutFormat(pdv.formats, args[0], args[1].Str)), nil
	case "input":
		// input(string, informat.) -> the string read through the informat (user
		// INVALUE informats and built-in informats both apply).
		if len(args) < 2 {
			return table.MissingNum(), fmt.Errorf("input expects 2 arguments, got %d", len(args))
		}
		field := args[0].Str
		if args[0].IsMissing() {
			field = ""
		}
		return applyInputInformat(pdv.informats, field, args[1].Str), nil
	default:
		return table.MissingNum(), fmt.Errorf("unknown function %q", e.Func)
	}
}

// aggregate sums/combines the non-missing numeric arguments starting from init.
// If all arguments are missing the result is missing (SAS sum of all-missing is
// missing).
func aggregate(args []table.Value, init float64, f func(acc, x float64) float64) table.Value {
	acc := init
	any := false
	for _, a := range args {
		if a.IsMissing() {
			continue
		}
		acc = f(acc, a.Num)
		any = true
	}
	if !any {
		return table.MissingNum()
	}
	return table.Num(acc)
}

// reduce combines non-missing numeric arguments without an identity, seeding from
// the first non-missing value (for min/max).
func reduce(args []table.Value, f func(acc, x float64) float64) table.Value {
	acc := 0.0
	any := false
	for _, a := range args {
		if a.IsMissing() {
			continue
		}
		if !any {
			acc = a.Num
			any = true
			continue
		}
		acc = f(acc, a.Num)
	}
	if !any {
		return table.MissingNum()
	}
	return table.Num(acc)
}

func meanFn(args []table.Value) table.Value {
	sum := aggregate(args, 0, func(acc, x float64) float64 { return acc + x })
	if sum.IsMissing() {
		return table.MissingNum()
	}
	return table.Num(sum.Num / float64(countNonMissing(args)))
}

func countNonMissing(args []table.Value) int {
	n := 0
	for _, a := range args {
		if !a.IsMissing() {
			n++
		}
	}
	return n
}

// scalar1 applies a unary math function, propagating missing.
func scalar1(args []table.Value, f func(float64) float64) (table.Value, error) {
	if len(args) != 1 {
		return table.MissingNum(), fmt.Errorf("function expects 1 argument, got %d", len(args))
	}
	if args[0].IsMissing() {
		return table.MissingNum(), nil
	}
	return table.Num(f(args[0].Num)), nil
}

func scalarStr(args []table.Value, f func(string) string) (table.Value, error) {
	if len(args) != 1 {
		return table.MissingNum(), fmt.Errorf("function expects 1 argument, got %d", len(args))
	}
	return table.Char(f(args[0].Str)), nil
}

// roundFn rounds the first argument to a multiple of the second (default 1),
// propagating missing.
func roundFn(args []table.Value) (table.Value, error) {
	if len(args) != 1 && len(args) != 2 {
		return table.MissingNum(), fmt.Errorf("round expects 1 or 2 arguments, got %d", len(args))
	}
	if args[0].IsMissing() {
		return table.MissingNum(), nil
	}
	unit := 1.0
	if len(args) == 2 {
		if args[1].IsMissing() || args[1].Num == 0 {
			return table.Num(args[0].Num), nil
		}
		unit = args[1].Num
	}
	return table.Num(math.Round(args[0].Num/unit) * unit), nil
}

// lengthFn returns the length of a character value; per SAS, an empty string has
// length 1.
func lengthFn(args []table.Value) table.Value {
	if len(args) != 1 {
		return table.MissingNum()
	}
	s := strings.TrimRight(args[0].Str, " ")
	if s == "" {
		return table.Num(1)
	}
	return table.Num(float64(len(s)))
}

// substrFn implements substr(s, pos[, len]) with 1-based positions.
func substrFn(args []table.Value) (table.Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return table.MissingNum(), fmt.Errorf("substr expects 2 or 3 arguments, got %d", len(args))
	}
	s := args[0].Str
	pos := int(args[1].Num)
	if pos < 1 {
		pos = 1
	}
	if pos > len(s) {
		return table.Char(""), nil
	}
	start := pos - 1
	end := len(s)
	if len(args) == 3 {
		n := int(args[2].Num)
		if n < 0 {
			n = 0
		}
		if start+n < end {
			end = start + n
		}
	}
	return table.Char(s[start:end]), nil
}

// catsFn concatenates its arguments after stripping leading/trailing blanks.
func catsFn(args []table.Value) table.Value {
	var b strings.Builder
	for _, a := range args {
		b.WriteString(strings.TrimSpace(a.Display()))
	}
	return table.Char(b.String())
}

// catxFn concatenates the 2nd..nth arguments, each stripped, joined by the first
// argument used as a separator (empty arguments are skipped, as in SAS).
func catxFn(args []table.Value) (table.Value, error) {
	if len(args) < 2 {
		return table.MissingNum(), fmt.Errorf("catx expects at least 2 arguments, got %d", len(args))
	}
	sep := args[0].Str
	var parts []string
	for _, a := range args[1:] {
		s := strings.TrimSpace(a.Display())
		if s != "" {
			parts = append(parts, s)
		}
	}
	return table.Char(strings.Join(parts, sep)), nil
}

// indexFn returns the 1-based position of the first occurrence of substring in
// string at or after start (0-based offset), or 0 if not found.
func indexFn(args []table.Value, start int) (table.Value, error) {
	if len(args) != 2 {
		return table.MissingNum(), fmt.Errorf("index expects 2 arguments, got %d", len(args))
	}
	s := args[0].Str
	if start > len(s) {
		return table.Num(0), nil
	}
	pos := strings.Index(s[start:], args[1].Str)
	if pos < 0 {
		return table.Num(0), nil
	}
	return table.Num(float64(start + pos + 1)), nil
}

// findFn is find(string, substring [, startpos]) — like index with an optional
// 1-based start position.
func findFn(args []table.Value) (table.Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return table.MissingNum(), fmt.Errorf("find expects 2 or 3 arguments, got %d", len(args))
	}
	start := 0
	if len(args) == 3 {
		if p := int(args[2].Num); p > 1 {
			start = p - 1
		}
	}
	return indexFn(args[:2], start)
}

// scanFn returns the nth word of a string. n<0 counts from the end. Consecutive
// delimiters are treated as one. The default delimiter set is a blank.
func scanFn(args []table.Value) (table.Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return table.MissingNum(), fmt.Errorf("scan expects 2 or 3 arguments, got %d", len(args))
	}
	delims := " "
	if len(args) == 3 {
		delims = args[2].Str
	}
	parts := strings.FieldsFunc(args[0].Str, func(r rune) bool {
		return strings.ContainsRune(delims, r)
	})
	n := int(args[1].Num)
	if n < 0 {
		n = len(parts) + n + 1
	}
	if n < 1 || n > len(parts) {
		return table.Char(""), nil
	}
	return table.Char(parts[n-1]), nil
}

// compressFn removes characters from a string. With one argument it removes
// blanks; with two it removes every character that appears in the second.
func compressFn(args []table.Value) (table.Value, error) {
	if len(args) != 1 && len(args) != 2 {
		return table.MissingNum(), fmt.Errorf("compress expects 1 or 2 arguments, got %d", len(args))
	}
	remove := " "
	if len(args) == 2 {
		remove = args[1].Str
	}
	out := strings.Map(func(r rune) rune {
		if strings.ContainsRune(remove, r) {
			return -1
		}
		return r
	}, args[0].Str)
	return table.Char(out), nil
}

// tranwrdFn replaces every occurrence of one substring with another.
func tranwrdFn(args []table.Value) (table.Value, error) {
	if len(args) != 3 {
		return table.MissingNum(), fmt.Errorf("tranwrd expects 3 arguments, got %d", len(args))
	}
	return table.Char(strings.ReplaceAll(args[0].Str, args[1].Str, args[2].Str)), nil
}

// propcase upper-cases the first letter of each word (a word starts after a
// blank) and lower-cases the rest.
func propcase(s string) string {
	b := []byte(strings.ToLower(s))
	atStart := true
	for i, c := range b {
		if c == ' ' {
			atStart = true
			continue
		}
		if atStart && c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
		atStart = false
	}
	return string(b)
}

// reverse returns the string with its bytes reversed.
func reverse(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

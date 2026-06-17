package runtime

import (
	"fmt"
	"math"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/table"
)

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

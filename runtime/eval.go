package runtime

import (
	"fmt"
	"math"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/table"
)

// Eval evaluates an expression against the given PDV, returning a table.Value.
// It implements SAS semantics: missing-value propagation in arithmetic,
// comparisons that yield 1/0, logical operators that treat missing and zero as
// false, string concatenation, and a core set of functions.
func Eval(expr ast.Expression, pdv *PDV) (table.Value, error) {
	switch e := expr.(type) {
	case *ast.NumberLiteral:
		return table.Num(e.Value), nil
	case *ast.StringLiteral:
		return table.Char(e.Value), nil
	case *ast.MissingLiteral:
		return table.MissingNum(), nil
	case *ast.Identifier:
		return pdv.Get(e.Name), nil
	case *ast.PrefixExpression:
		return evalPrefix(e, pdv)
	case *ast.InfixExpression:
		return evalInfix(e, pdv)
	case *ast.CallExpression:
		return evalCall(e, pdv)
	default:
		return table.MissingNum(), fmt.Errorf("cannot evaluate expression %T", expr)
	}
}

func evalPrefix(e *ast.PrefixExpression, pdv *PDV) (table.Value, error) {
	right, err := Eval(e.Right, pdv)
	if err != nil {
		return table.MissingNum(), err
	}
	switch strings.ToLower(e.Op) {
	case "-":
		if right.IsMissing() {
			return table.MissingNum(), nil
		}
		return table.Num(-right.Num), nil
	case "+":
		if right.IsMissing() {
			return table.MissingNum(), nil
		}
		return table.Num(right.Num), nil
	case "not", "^", "~":
		return boolVal(!truthy(right)), nil
	default:
		return table.MissingNum(), fmt.Errorf("unknown prefix operator %q", e.Op)
	}
}

func evalInfix(e *ast.InfixExpression, pdv *PDV) (table.Value, error) {
	op := strings.ToLower(e.Op)

	// Logical operators short-circuit and operate on truthiness.
	switch op {
	case "and", "&":
		l, err := Eval(e.Left, pdv)
		if err != nil {
			return table.MissingNum(), err
		}
		if !truthy(l) {
			return boolVal(false), nil
		}
		r, err := Eval(e.Right, pdv)
		if err != nil {
			return table.MissingNum(), err
		}
		return boolVal(truthy(r)), nil
	case "or", "|":
		l, err := Eval(e.Left, pdv)
		if err != nil {
			return table.MissingNum(), err
		}
		if truthy(l) {
			return boolVal(true), nil
		}
		r, err := Eval(e.Right, pdv)
		if err != nil {
			return table.MissingNum(), err
		}
		return boolVal(truthy(r)), nil
	}

	left, err := Eval(e.Left, pdv)
	if err != nil {
		return table.MissingNum(), err
	}
	right, err := Eval(e.Right, pdv)
	if err != nil {
		return table.MissingNum(), err
	}

	switch op {
	case "||", "!!":
		return table.Char(left.Str + right.Str), nil
	case "+", "-", "*", "/", "**":
		return evalArith(op, left, right)
	case "=", "eq", "ne", "^=", "~=", "<", "lt", "<=", "le", ">", "gt", ">=", "ge":
		return boolVal(compare(op, left, right)), nil
	default:
		return table.MissingNum(), fmt.Errorf("unknown operator %q", e.Op)
	}
}

func evalArith(op string, left, right table.Value) (table.Value, error) {
	// Missing propagates through arithmetic.
	if left.IsMissing() || right.IsMissing() {
		return table.MissingNum(), nil
	}
	a, b := left.Num, right.Num
	switch op {
	case "+":
		return table.Num(a + b), nil
	case "-":
		return table.Num(a - b), nil
	case "*":
		return table.Num(a * b), nil
	case "/":
		if b == 0 {
			// SAS notes division by zero and yields missing.
			return table.MissingNum(), nil
		}
		return table.Num(a / b), nil
	case "**":
		return table.Num(math.Pow(a, b)), nil
	}
	return table.MissingNum(), fmt.Errorf("unknown arithmetic operator %q", op)
}

// compare implements SAS comparison using the canonical value ordering (see
// table.Value.Compare): character values compare lexically; numeric values
// compare by magnitude with missing ordered below every number.
func compare(op string, left, right table.Value) bool {
	cmp := left.Compare(right)
	switch op {
	case "=", "eq":
		return cmp == 0
	case "ne", "^=", "~=":
		return cmp != 0
	case "<", "lt":
		return cmp < 0
	case "<=", "le":
		return cmp <= 0
	case ">", "gt":
		return cmp > 0
	case ">=", "ge":
		return cmp >= 0
	}
	return false
}

// truthy reports whether a value is "true" in a logical context: a non-missing,
// non-zero number. Character and missing values are false.
func truthy(v table.Value) bool {
	if v.Kind == table.Character {
		return v.Str != ""
	}
	return !v.IsMissing() && v.Num != 0
}

// boolVal converts a Go bool to the SAS numeric 1/0.
func boolVal(b bool) table.Value {
	if b {
		return table.Num(1)
	}
	return table.Num(0)
}

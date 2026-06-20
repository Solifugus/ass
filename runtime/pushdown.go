package runtime

import (
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/table"
)

// pushdownSelection derives a value-safe pushdown Selection from a source's
// dataset options, so an external (database) source can be asked to project
// columns and filter rows itself instead of transferring the whole table. It is
// purely an optimization: the caller still applies the full dataset options
// locally (applyDatasetOptions), so whatever is pushed only reduces transfer —
// it never changes results. Accordingly this is conservative, pushing only what
// provably matches SAS:
//
//   - KEEP= becomes a column projection (DROP= alone is not pushed — expressing
//     "all but" needs the full schema, which the runtime does not have here).
//     The columns a WHERE references are always added so a locally-applied WHERE
//     still has them.
//   - WHERE= is translated to a Filter only for the subset of comparisons whose
//     SQL row selection matches SAS exactly (see translateFilter); anything else
//     is left to the local pass.
func pushdownSelection(opts *ast.DatasetOptions) table.Selection {
	if opts == nil || opts.IsEmpty() {
		return table.Selection{}
	}
	var sel table.Selection

	if len(opts.Keep) > 0 {
		seen := map[string]bool{}
		var order []string
		add := func(n string) {
			ln := strings.ToLower(n)
			if !seen[ln] {
				seen[ln] = true
				order = append(order, n)
			}
		}
		for _, k := range opts.Keep {
			add(k)
		}
		if opts.Where != nil {
			for _, c := range whereColumns(opts.Where) {
				add(c)
			}
		}
		sel.Columns = order
	}

	if opts.Where != nil {
		sel.Filter = translateFilter(opts.Where)
	}
	return sel
}

// whereColumns collects the variable names a WHERE expression references, so they
// can be retained by a pushed-down projection even when KEEP= would otherwise
// drop them (SAS evaluates an input WHERE before KEEP takes effect).
func whereColumns(e ast.Expression) []string {
	var out []string
	var walk func(ast.Expression)
	walk = func(n ast.Expression) {
		switch x := n.(type) {
		case *ast.Identifier:
			out = append(out, x.Name)
		case *ast.InfixExpression:
			walk(x.Left)
			walk(x.Right)
		case *ast.PrefixExpression:
			walk(x.Right)
		case *ast.CallExpression:
			for _, a := range x.Args {
				walk(a)
			}
		case *ast.ArrayRef:
			out = append(out, x.Name)
			walk(x.Index)
		}
	}
	walk(e)
	return out
}

// translateFilter converts a WHERE expression to a dialect-neutral table.Filter,
// or nil if it cannot be safely pushed. It accepts AND/OR of numeric
// column-vs-literal comparisons using only =, >, >= (after normalizing a
// literal-on-the-left form). Operators that keep missing in SAS (<, <=, ne), NOT,
// string comparisons, and anything involving functions or column-to-column
// comparisons are rejected (returns nil), leaving them to the local WHERE pass.
func translateFilter(e ast.Expression) *table.Filter {
	n, ok := e.(*ast.InfixExpression)
	if !ok {
		return nil
	}
	switch strings.ToLower(n.Op) {
	case "and", "&":
		l, r := translateFilter(n.Left), translateFilter(n.Right)
		if l == nil || r == nil {
			return nil
		}
		return &table.Filter{Kind: table.FilterAnd, Sub: []*table.Filter{l, r}}
	case "or", "|":
		l, r := translateFilter(n.Left), translateFilter(n.Right)
		if l == nil || r == nil {
			return nil
		}
		return &table.Filter{Kind: table.FilterOr, Sub: []*table.Filter{l, r}}
	default:
		return translateCmp(n)
	}
}

// translateCmp handles a single comparison, normalizing `literal OP column` to
// `column OP literal` (flipping the operator) before checking it is a safe one.
func translateCmp(n *ast.InfixExpression) *table.Filter {
	op := strings.ToLower(n.Op)
	if col, ok := n.Left.(*ast.Identifier); ok {
		if num, ok := numLit(n.Right); ok {
			return cmpFilter(col.Name, op, num)
		}
		return nil
	}
	if col, ok := n.Right.(*ast.Identifier); ok {
		if num, ok := numLit(n.Left); ok {
			return cmpFilter(col.Name, flipOp(op), num)
		}
		return nil
	}
	return nil
}

// numLit extracts a numeric literal (optionally signed) from an expression.
func numLit(e ast.Expression) (float64, bool) {
	switch x := e.(type) {
	case *ast.NumberLiteral:
		return x.Value, true
	case *ast.PrefixExpression:
		if v, ok := numLit(x.Right); ok {
			switch x.Op {
			case "-":
				return -v, true
			case "+":
				return v, true
			}
		}
	}
	return 0, false
}

// flipOp returns the operator equivalent when the operands are swapped, so a
// `literal OP column` comparison can be normalized to `column OP' literal`.
func flipOp(op string) string {
	switch op {
	case "<", "lt":
		return ">"
	case "<=", "le":
		return ">="
	case ">", "gt":
		return "<"
	case ">=", "ge":
		return "<="
	case "=", "eq":
		return "="
	}
	return op // ne / ^= etc. — left as-is and rejected by cmpFilter
}

// cmpFilter builds a leaf Filter only for the safe operators (=, >, >=), which
// exclude missing in SAS exactly as SQL's NULL handling excludes NULL. Any other
// operator (<, <=, ne — all of which keep missing in SAS) yields nil so the
// comparison is filtered locally instead.
func cmpFilter(col, op string, num float64) *table.Filter {
	var canon string
	switch op {
	case "=", "eq":
		canon = "="
	case ">", "gt":
		canon = ">"
	case ">=", "ge":
		canon = ">="
	default:
		return nil
	}
	return &table.Filter{Kind: table.FilterCmp, Column: col, Op: canon, Number: num}
}

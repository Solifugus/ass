package runtime

import (
	"strconv"
	"testing"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/table"
)

// pwhere parses a bare WHERE expression in isolation.
func pwhere(src string) ast.Expression { return parser.ParseExpressionString(src) }

// TestTranslateFilterSafeOps confirms only the value-safe operators (=, >, >=)
// are pushed, including the literal-on-the-left normalization, and that unsafe
// forms are left to the local pass.
func TestTranslateFilterSafeOps(t *testing.T) {
	cases := []struct {
		expr string
		want string // canonical "col op num", or "" if not translatable
	}{
		{"age >= 18", "age >= 18"},
		{"age > 18", "age > 18"},
		{"age = 18", "age = 18"},
		{"18 < age", "age > 18"},   // flipped
		{"18 <= age", "age >= 18"}, // flipped
		{"age < 18", ""},           // keeps missing in SAS — never pushed
		{"age <= 18", ""},
		{"age ne 18", ""},
		{"age ^= 18", ""},
		{"not (age >= 18)", ""}, // NOT not pushed
		{"name = 'foo'", ""},    // string comparison not pushed
		{"age >= -5", "age >= -5"},
	}
	for _, c := range cases {
		got := filterString(translateFilter(pwhere(c.expr)))
		if got != c.want {
			t.Errorf("%q -> %q, want %q", c.expr, got, c.want)
		}
	}
}

// TestTranslateFilterAndOr checks boolean composition: a whole AND/OR is pushed
// only if every leaf is safe.
func TestTranslateFilterAndOr(t *testing.T) {
	if got := filterString(translateFilter(pwhere("age >= 18 and salary > 1000"))); got != "(age >= 18 AND salary > 1000)" {
		t.Errorf("and: got %q", got)
	}
	if got := filterString(translateFilter(pwhere("age >= 18 or salary > 1000"))); got != "(age >= 18 OR salary > 1000)" {
		t.Errorf("or: got %q", got)
	}
	// One unsafe leaf poisons the whole expression (left to the local pass).
	if f := translateFilter(pwhere("age >= 18 and salary < 1000")); f != nil {
		t.Errorf("mixed and: got %q, want nil", filterString(f))
	}
}

// TestPushdownSelectionKeepIncludesWhereCols verifies a KEEP projection also
// carries the WHERE's columns (so a locally applied WHERE still sees them), and
// that DROP= alone is not pushed as a projection.
func TestPushdownSelectionKeepIncludesWhereCols(t *testing.T) {
	opts := &ast.DatasetOptions{Keep: []string{"a", "b"}, Where: pwhere("c > 5")}
	sel := pushdownSelection(opts)
	for _, want := range []string{"a", "b", "c"} {
		if !contains(sel.Columns, want) {
			t.Errorf("projection = %v, missing %q", sel.Columns, want)
		}
	}

	dropOnly := pushdownSelection(&ast.DatasetOptions{Drop: []string{"a"}})
	if len(dropOnly.Columns) != 0 {
		t.Errorf("DROP= alone should not push a projection, got %v", dropOnly.Columns)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// filterString renders a Filter back to a stable canonical string for assertions.
func filterString(f *table.Filter) string {
	if f == nil {
		return ""
	}
	switch f.Kind {
	case table.FilterCmp:
		return f.Column + " " + f.Op + " " + strconv.FormatFloat(f.Number, 'g', -1, 64)
	case table.FilterAnd, table.FilterOr:
		sep := " AND "
		if f.Kind == table.FilterOr {
			sep = " OR "
		}
		out := "("
		for i, s := range f.Sub {
			if i > 0 {
				out += sep
			}
			out += filterString(s)
		}
		return out + ")"
	}
	return ""
}

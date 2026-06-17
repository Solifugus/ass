package runtime

import (
	"testing"

	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/table"
)

// evalExpr parses a SAS expression string and evaluates it against pdv.
func evalExpr(t *testing.T, src string, pdv *PDV) table.Value {
	t.Helper()
	expr := parser.ParseExpressionString(src)
	if expr == nil {
		t.Fatalf("failed to parse expression %q", src)
	}
	v, err := Eval(expr, pdv)
	if err != nil {
		t.Fatalf("Eval(%q) error: %v", src, err)
	}
	return v
}

func TestEvalArithmetic(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("x", table.Num(10))
	pdv.Set("y", table.Num(4))
	cases := []struct {
		src  string
		want float64
	}{
		{"2 + 3", 5},
		{"x - y", 6},
		{"x * y", 40},
		{"x / y", 2.5},
		{"2 ** 3", 8},
		{"-x", -10},
		{"x + y * 2", 18}, // precedence
	}
	for _, c := range cases {
		got := evalExpr(t, c.src, pdv)
		if got.IsMissing() || got.Num != c.want {
			t.Errorf("%s = %v, want %v", c.src, got.Display(), c.want)
		}
	}
}

func TestEvalMissingPropagation(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("m", table.MissingNum())
	pdv.Set("x", table.Num(5))
	for _, src := range []string{"m + x", "m * x", "m - 1", "-m", "x / 0"} {
		if got := evalExpr(t, src, pdv); !got.IsMissing() {
			t.Errorf("%s = %v, want missing", src, got.Display())
		}
	}
}

func TestEvalComparison(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("age", table.Num(18))
	pdv.Set("name", table.Char("John"))
	cases := []struct {
		src  string
		want float64
	}{
		{"age >= 18", 1},
		{"age > 18", 0},
		{"age = 18", 1},
		{"age ne 20", 1},
		{"age lt 20", 1},
		{"name = 'John'", 1},
		{"name = 'Jane'", 0},
		{"'abc' < 'abd'", 1},
	}
	for _, c := range cases {
		got := evalExpr(t, c.src, pdv)
		if got.Num != c.want {
			t.Errorf("%s = %v, want %v", c.src, got.Display(), c.want)
		}
	}
}

func TestEvalMissingOrdersBelowNumbers(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("m", table.MissingNum())
	if got := evalExpr(t, "m < 0", pdv); got.Num != 1 {
		t.Errorf("missing < 0 = %v, want 1 (missing sorts low)", got.Display())
	}
	if got := evalExpr(t, "m < -1000000", pdv); got.Num != 1 {
		t.Errorf("missing < -1e6 = %v, want 1", got.Display())
	}
}

func TestEvalLogical(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("a", table.Num(1))
	pdv.Set("b", table.Num(0))
	pdv.Set("m", table.MissingNum())
	cases := []struct {
		src  string
		want float64
	}{
		{"a and a", 1},
		{"a and b", 0},
		{"a or b", 1},
		{"b or b", 0},
		{"not b", 1},
		{"not a", 0},
		{"m and a", 0}, // missing is false
		{"a and (age > 5)", 0},
	}
	pdv.Set("age", table.Num(3))
	for _, c := range cases {
		got := evalExpr(t, c.src, pdv)
		if got.Num != c.want {
			t.Errorf("%s = %v, want %v", c.src, got.Display(), c.want)
		}
	}
}

func TestEvalConcat(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("first", table.Char("John"))
	pdv.Set("last", table.Char("Doe"))
	got := evalExpr(t, "first || ' ' || last", pdv)
	if got.Str != "John Doe" {
		t.Errorf("concat = %q, want %q", got.Str, "John Doe")
	}
}

func TestEvalFunctions(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("x", table.Num(3))
	pdv.Set("m", table.MissingNum())
	cases := []struct {
		src  string
		want float64
	}{
		{"sum(1, 2, 3)", 6},
		{"sum(1, m, 3)", 4}, // ignores missing
		{"mean(2, 4, 6)", 4},
		{"min(5, 2, 8)", 2},
		{"max(5, 2, 8)", 8},
		{"n(1, m, 3)", 2},
		{"abs(-7)", 7},
		{"int(3.9)", 3},
		{"round(3.14159, 0.01)", 3.14},
		{"round(2.5)", 3},
		{"ceil(2.1)", 3},
		{"floor(2.9)", 2},
	}
	for _, c := range cases {
		got := evalExpr(t, c.src, pdv)
		if got.IsMissing() || got.Num != c.want {
			t.Errorf("%s = %v, want %v", c.src, got.Display(), c.want)
		}
	}
}

func TestEvalStringFunctions(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("name", table.Char("John"))
	cases := []struct {
		src  string
		want string
	}{
		{"upcase(name)", "JOHN"},
		{"lowcase('ABC')", "abc"},
		{"substr('abcdef', 2, 3)", "bcd"},
		{"substr('abcdef', 4)", "def"},
		{"trim('hi   ')", "hi"},
		{"cats('a', 'b', 'c')", "abc"},
	}
	for _, c := range cases {
		got := evalExpr(t, c.src, pdv)
		if got.Str != c.want {
			t.Errorf("%s = %q, want %q", c.src, got.Str, c.want)
		}
	}
}

func TestEvalMoreStringFunctions(t *testing.T) {
	pdv := NewPDV()
	cases := []struct {
		src  string
		want string
	}{
		{"scan('a,b,c', 2, ',')", "b"},
		{"scan('one two three', 3)", "three"},
		{"scan('one two', -1)", "two"},
		{"compress('a b c')", "abc"},
		{"compress('a-b-c', '-')", "abc"},
		{"tranwrd('a cat sat', 'a', 'A')", "A cAt sAt"},
		{"propcase('john q public')", "John Q Public"},
		{"reverse('abc')", "cba"},
		{"catx('-', 'a', 'b', 'c')", "a-b-c"},
		{"catx('-', 'a', '', 'c')", "a-c"},
	}
	for _, c := range cases {
		got := evalExpr(t, c.src, pdv)
		if got.Str != c.want {
			t.Errorf("%s = %q, want %q", c.src, got.Str, c.want)
		}
	}
}

func TestEvalNumericLookupFunctions(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("m", table.MissingNum())
	cases := []struct {
		src  string
		want float64
	}{
		{"index('hello', 'l')", 3},
		{"index('hello', 'z')", 0},
		{"find('abcabc', 'bc', 3)", 5},
		{"missing(m)", 1},
		{"missing(5)", 0},
	}
	for _, c := range cases {
		got := evalExpr(t, c.src, pdv)
		if got.IsMissing() || got.Num != c.want {
			t.Errorf("%s = %v, want %v", c.src, got.Display(), c.want)
		}
	}
}

func TestEvalCharToNumCoercion(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("s", table.Char("42"))
	pdv.Set("bad", table.Char("xyz"))
	if got := evalExpr(t, "s + 8", pdv); got.IsMissing() || got.Num != 50 {
		t.Errorf("'42' + 8 = %v, want 50", got.Display())
	}
	if got := evalExpr(t, "bad * 2", pdv); !got.IsMissing() {
		t.Errorf("'xyz' * 2 = %v, want missing", got.Display())
	}
}

func TestEvalNumToCharCoercionInConcat(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("age", table.Num(25))
	got := evalExpr(t, "'age=' || age", pdv)
	if got.Str != "age=25" {
		t.Errorf("concat with numeric = %q, want %q", got.Str, "age=25")
	}
}

func TestEvalMixedComparison(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("n", table.Num(5))
	// Character literal compared to a numeric variable coerces to numeric.
	if got := evalExpr(t, "n = '5'", pdv); got.Num != 1 {
		t.Errorf("5 = '5' (coerced) = %v, want 1", got.Display())
	}
	if got := evalExpr(t, "n > '10'", pdv); got.Num != 0 {
		t.Errorf("5 > '10' (coerced) = %v, want 0", got.Display())
	}
}

func TestEvalSumAllMissingIsMissing(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("m", table.MissingNum())
	if got := evalExpr(t, "sum(m, m)", pdv); !got.IsMissing() {
		t.Errorf("sum of all missing = %v, want missing", got.Display())
	}
}

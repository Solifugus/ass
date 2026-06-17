package parser

import "testing"

// parseExpr is a test helper that parses a single expression from src.
func parseExpr(t *testing.T, src string) string {
	t.Helper()
	p := New(src)
	e := p.parseExpression(pLOWEST)
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parse errors for %q: %v", src, errs)
	}
	if e == nil {
		t.Fatalf("nil expression for %q", src)
	}
	return e.String()
}

func TestExpressionPrecedence(t *testing.T) {
	cases := map[string]string{
		"a + b * c":      "(a + (b * c))",
		"a * b + c":      "((a * b) + c)",
		"a + b + c":      "((a + b) + c)",
		"(a + b) * c":    "((a + b) * c)",
		"a = b + c":      "(a = (b + c))",
		"a and b or c":   "((a and b) or c)",
		"a or b and c":   "(a or (b and c))",
		"age >= 18":      "(age >= 18)",
		"x ** 2 ** 3":    "(x ** (2 ** 3))", // right associative
		"-2 ** 2":        "(-(2 ** 2))",     // ** binds tighter than unary minus
		"a || b || c":    "((a || b) || c)",
		"not a and b":    "((not a) and b)",
		"not a = b":      "(not (a = b))", // NOT looser than comparison
	}
	for src, want := range cases {
		if got := parseExpr(t, src); got != want {
			t.Errorf("%q => %s, want %s", src, got, want)
		}
	}
}

func TestMnemonicOperators(t *testing.T) {
	cases := map[string]string{
		"a eq b": "(a = b)",
		"a ne b": "(a ^= b)",
		"a lt b": "(a < b)",
		"a ge b": "(a >= b)",
	}
	for src, want := range cases {
		if got := parseExpr(t, src); got != want {
			t.Errorf("%q => %s, want %s", src, got, want)
		}
	}
}

func TestFunctionCalls(t *testing.T) {
	cases := map[string]string{
		"upcase(name)":          "upcase(name)",
		"substr(s, 1, 3)":       "substr(s, 1, 3)",
		"sum(a, b, c)":          "sum(a, b, c)",
		"round(x * 2, 0.1)":     "round((x * 2), 0.1)",
		"max(a, min(b, c))":     "max(a, min(b, c))",
	}
	for src, want := range cases {
		if got := parseExpr(t, src); got != want {
			t.Errorf("%q => %s, want %s", src, got, want)
		}
	}
}

func TestLiterals(t *testing.T) {
	if got := parseExpr(t, "'hello'"); got != "'hello'" {
		t.Errorf("string literal => %s", got)
	}
	if got := parseExpr(t, "."); got != "." {
		t.Errorf("missing literal => %s", got)
	}
	if got := parseExpr(t, "3.14"); got != "3.14" {
		t.Errorf("number literal => %s", got)
	}
}

package parser

import (
	"testing"

	"github.com/solifugus/ass/ast"
)

func TestExpandVarRange(t *testing.T) {
	cases := []struct {
		lo, hi string
		want   []string
	}{
		{"x1", "x5", []string{"x1", "x2", "x3", "x4", "x5"}},
		{"x01", "x03", []string{"x01", "x02", "x03"}}, // zero-pad to lo width
		{"var10", "var12", []string{"var10", "var11", "var12"}},
		{"x3", "x3", []string{"x3"}},
		{"a", "b", []string{"a", "b"}},     // no digits -> not a range
		{"x5", "x1", []string{"x5", "x1"}}, // descending -> not a range
		{"x1", "y3", []string{"x1", "y3"}}, // prefix mismatch -> not a range
	}
	for _, c := range cases {
		got := expandVarRange(c.lo, c.hi)
		if len(got) != len(c.want) {
			t.Errorf("expandVarRange(%q,%q) = %v, want %v", c.lo, c.hi, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("expandVarRange(%q,%q) = %v, want %v", c.lo, c.hi, got, c.want)
				break
			}
		}
	}
}

func TestParseKeepStatementRange(t *testing.T) {
	body := dataBody(t, "data s; set in; keep id x1-x3 name; run;")
	var ks *ast.KeepStatement
	for _, st := range body {
		if k, ok := st.(*ast.KeepStatement); ok {
			ks = k
		}
	}
	if ks == nil {
		t.Fatal("no KeepStatement parsed")
	}
	want := []string{"id", "x1", "x2", "x3", "name"}
	if len(ks.Vars) != len(want) {
		t.Fatalf("keep vars = %v, want %v", ks.Vars, want)
	}
	for i, w := range want {
		if ks.Vars[i] != w {
			t.Errorf("keep var %d = %q, want %q (%v)", i, ks.Vars[i], w, ks.Vars)
		}
	}
}

func TestParseKeepOptionRange(t *testing.T) {
	body := dataBody(t, "data s; set in(keep=id x1-x3); run;")
	var ss *ast.SetStatement
	for _, st := range body {
		if s, ok := st.(*ast.SetStatement); ok {
			ss = s
		}
	}
	if ss == nil || len(ss.Refs) == 0 || ss.Refs[0].Options == nil {
		t.Fatal("no SET options parsed")
	}
	want := []string{"id", "x1", "x2", "x3"}
	got := ss.Refs[0].Options.Keep
	if len(got) != len(want) {
		t.Fatalf("keep= = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("keep= var %d = %q, want %q (%v)", i, got[i], w, got)
		}
	}
}

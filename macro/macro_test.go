package macro

import (
	"strings"
	"testing"
)

func TestLetAndResolve(t *testing.T) {
	out := Process("%let cutoff = 18;\nif age >= &cutoff;")
	if strings.Contains(out, "%let") {
		t.Errorf("%%let should be consumed; out=%q", out)
	}
	if !strings.Contains(out, "if age >= 18;") {
		t.Errorf("&cutoff not resolved; out=%q", out)
	}
}

func TestResolveTrailingDot(t *testing.T) {
	out := Process("%let lib = work;\ndata &lib..thing;")
	if !strings.Contains(out, "data work.thing;") {
		t.Errorf("trailing-dot resolution wrong; out=%q", out)
	}
}

func TestNestedLetReference(t *testing.T) {
	// A %let value may reference an earlier macro variable.
	out := Process("%let a = 5;\n%let b = &a;\nx = &b;")
	if !strings.Contains(out, "x = 5;") {
		t.Errorf("nested let reference wrong; out=%q", out)
	}
}

func TestMacroPositionalParam(t *testing.T) {
	src := "%macro show(ds);\n  proc print data=&ds;\n  run;\n%mend show;\n%show(people)"
	out := Process(src)
	if strings.Contains(out, "%macro") || strings.Contains(out, "%mend") {
		t.Errorf("definition should be consumed; out=%q", out)
	}
	if !strings.Contains(out, "proc print data=people;") {
		t.Errorf("macro call did not expand; out=%q", out)
	}
}

func TestMacroNamedParamDefault(t *testing.T) {
	// x is a keyword parameter (has a default); it is set by name or defaults.
	src := "%macro f(x=7);\nval=&x;\n%mend;\n%f()\n%f(x=9)"
	out := Process(src)
	if !strings.Contains(out, "val=7;") {
		t.Errorf("default param not used; out=%q", out)
	}
	if !strings.Contains(out, "val=9;") {
		t.Errorf("explicit keyword param not used; out=%q", out)
	}
}

func TestMacroDoLoop(t *testing.T) {
	src := "%macro gen(n);\n  data nums;\n    %do i = 1 %to &n;\n      x = &i;\n      output;\n    %end;\n  run;\n%mend gen;\n%gen(3)"
	out := Process(src)
	for _, want := range []string{"data nums;", "x = 1;", "x = 2;", "x = 3;", "run;"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in expansion; out=%q", want, out)
		}
	}
	if strings.Contains(out, "x = 4;") {
		t.Errorf("loop overran; out=%q", out)
	}
}

func TestMacroIfThenElse(t *testing.T) {
	def := "%macro c(n);\n%if &n > 0 %then %do;\npos=1;\n%end;\n%else %do;\nneg=1;\n%end;\n%mend c;\n"
	out := Process(def + "%c(5)")
	if !strings.Contains(out, "pos=1;") || strings.Contains(out, "neg=1;") {
		t.Errorf("then branch wrong for n=5; out=%q", out)
	}
	out = Process(def + "%c(-2)")
	if !strings.Contains(out, "neg=1;") || strings.Contains(out, "pos=1;") {
		t.Errorf("else branch wrong for n=-2; out=%q", out)
	}
}

func TestEvalCond(t *testing.T) {
	cases := []struct {
		expr string
		want bool
	}{
		{"5 > 0", true},
		{"5 < 0", false},
		{"3 = 3", true},
		{"3 ne 4", true},
		{"abc = abc", true},
		{"1", true},
		{"0", false},
		{"", false},
	}
	for _, c := range cases {
		if got := evalCond(c.expr); got != c.want {
			t.Errorf("evalCond(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

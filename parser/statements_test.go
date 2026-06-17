package parser

import (
	"testing"

	"github.com/solifugus/ass/ast"
)

// dataBody parses a single DATA step and returns its body statements.
func dataBody(t *testing.T, src string) []ast.Statement {
	t.Helper()
	p := New(src)
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parse errors for %q: %v", src, errs)
	}
	ds, ok := prog.Steps[0].(*ast.DataStep)
	if !ok {
		t.Fatalf("step 0 is %T, want *ast.DataStep", prog.Steps[0])
	}
	return ds.Body
}

func TestParseAssignmentAndInput(t *testing.T) {
	body := dataBody(t, "data s; input item $ qty price; total = qty * price; run;")
	if len(body) != 2 {
		t.Fatalf("got %d statements, want 2: %v", len(body), body)
	}
	in, ok := body[0].(*ast.InputStatement)
	if !ok {
		t.Fatalf("stmt 0 is %T, want InputStatement", body[0])
	}
	if len(in.Vars) != 3 || !in.Vars[0].Char || in.Vars[1].Char {
		t.Errorf("input vars wrong: %+v", in.Vars)
	}
	asg, ok := body[1].(*ast.AssignmentStatement)
	if !ok {
		t.Fatalf("stmt 1 is %T, want AssignmentStatement", body[1])
	}
	if asg.Name != "total" || asg.Value.String() != "(qty * price)" {
		t.Errorf("assignment = %s = %s", asg.Name, asg.Value.String())
	}
}

func TestParseSetAndSubsettingIf(t *testing.T) {
	body := dataBody(t, "data adults; set people; if age >= 18; run;")
	if _, ok := body[0].(*ast.SetStatement); !ok {
		t.Fatalf("stmt 0 is %T, want SetStatement", body[0])
	}
	si, ok := body[1].(*ast.SubsettingIf)
	if !ok {
		t.Fatalf("stmt 1 is %T, want SubsettingIf", body[1])
	}
	if si.Condition.String() != "(age >= 18)" {
		t.Errorf("subsetting cond = %s", si.Condition.String())
	}
}

func TestParseIfThenElse(t *testing.T) {
	body := dataBody(t, "data g; if s >= 90 then grade = 'A'; else grade = 'B'; run;")
	ifs, ok := body[0].(*ast.IfStatement)
	if !ok {
		t.Fatalf("stmt 0 is %T, want IfStatement", body[0])
	}
	if _, ok := ifs.Consequence.(*ast.AssignmentStatement); !ok {
		t.Errorf("consequence is %T, want AssignmentStatement", ifs.Consequence)
	}
	if ifs.Alternative == nil {
		t.Errorf("expected else branch")
	}
}

func TestParseDoLoopWithOutput(t *testing.T) {
	body := dataBody(t, "data sq; do i = 1 to 5; x = i * i; output; end; run;")
	do, ok := body[0].(*ast.DoStatement)
	if !ok {
		t.Fatalf("stmt 0 is %T, want DoStatement", body[0])
	}
	if do.Kind != ast.DoIterative || do.Var != "i" {
		t.Errorf("do kind/var = %d/%s", do.Kind, do.Var)
	}
	if do.From.String() != "1" || do.To.String() != "5" {
		t.Errorf("do range = %s..%s", do.From.String(), do.To.String())
	}
	if len(do.Body) != 2 {
		t.Fatalf("do body = %d statements, want 2", len(do.Body))
	}
	if _, ok := do.Body[1].(*ast.OutputStatement); !ok {
		t.Errorf("do body[1] is %T, want OutputStatement", do.Body[1])
	}
}

func TestParseProcByAndVar(t *testing.T) {
	p := New("proc sort data=people; by descending age; run; proc print data=people; var name age; run;")
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	sort := prog.Steps[0].(*ast.ProcStep)
	by, ok := sort.Body[0].(*ast.ByStatement)
	if !ok {
		t.Fatalf("sort body[0] is %T, want ByStatement", sort.Body[0])
	}
	if len(by.Vars) != 1 || by.Vars[0] != "age" || !by.Descending[0] {
		t.Errorf("by = %+v desc=%+v", by.Vars, by.Descending)
	}
	print := prog.Steps[1].(*ast.ProcStep)
	v, ok := print.Body[0].(*ast.VarStatement)
	if !ok {
		t.Fatalf("print body[0] is %T, want VarStatement", print.Body[0])
	}
	if len(v.Vars) != 2 {
		t.Errorf("var list = %v", v.Vars)
	}
}

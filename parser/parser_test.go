package parser

import (
	"testing"

	"github.com/solifugus/ass/ast"
)

func TestParseProgramStepCounts(t *testing.T) {
	input := `data people;
  input name $ age;
  datalines;
John 25
Mary 30
;
run;

proc print data=people;
  var name age;
run;`

	prog := New(input).ParseProgram()
	if len(prog.Steps) != 2 {
		t.Fatalf("got %d steps, want 2: %s", len(prog.Steps), prog.String())
	}

	ds, ok := prog.Steps[0].(*ast.DataStep)
	if !ok {
		t.Fatalf("step 0 is %T, want *ast.DataStep", prog.Steps[0])
	}
	if len(ds.Datasets) != 1 || ds.Datasets[0] != "people" {
		t.Errorf("data step datasets = %v, want [people]", ds.Datasets)
	}

	ps, ok := prog.Steps[1].(*ast.ProcStep)
	if !ok {
		t.Fatalf("step 1 is %T, want *ast.ProcStep", prog.Steps[1])
	}
	if ps.Name != "print" {
		t.Errorf("proc name = %q, want print", ps.Name)
	}
	if ps.Data != "people" {
		t.Errorf("proc data= = %q, want people", ps.Data)
	}
}

func TestParseDatalinesCaptured(t *testing.T) {
	input := "data d; input x; datalines;\n1\n2\n3\n;\nrun;"
	prog := New(input).ParseProgram()
	ds := prog.Steps[0].(*ast.DataStep)

	var dl *ast.DatalinesStatement
	for _, s := range ds.Body {
		if d, ok := s.(*ast.DatalinesStatement); ok {
			dl = d
		}
	}
	if dl == nil {
		t.Fatalf("no DatalinesStatement found in body: %v", ds.Body)
	}
	if len(dl.Lines) != 3 {
		t.Errorf("datalines lines = %v, want 3 lines", dl.Lines)
	}
}

func TestParseProcOptionsAndFlags(t *testing.T) {
	input := "proc sort data=people out=sorted nodupkey; by age; run;"
	prog := New(input).ParseProgram()
	ps := prog.Steps[0].(*ast.ProcStep)
	if ps.Name != "sort" || ps.Data != "people" {
		t.Fatalf("name/data = %q/%q, want sort/people", ps.Name, ps.Data)
	}
	// Expect out=sorted and a bare nodupkey flag.
	var foundOut, foundFlag bool
	for _, o := range ps.Options {
		if o.Name == "out" && o.Value == "sorted" {
			foundOut = true
		}
		if o.Name == "nodupkey" && o.Value == "" {
			foundFlag = true
		}
	}
	if !foundOut || !foundFlag {
		t.Errorf("options = %v, want out=sorted and nodupkey flag", ps.Options)
	}
}

func TestNoParseErrorsOnCleanInput(t *testing.T) {
	input := "data a; x = 1; run; proc print data=a; run;"
	p := New(input)
	p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Errorf("unexpected parse errors: %v", errs)
	}
}

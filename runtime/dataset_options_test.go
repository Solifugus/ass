package runtime

import (
	"strings"
	"testing"

	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/table"
)

func runSrc(t *testing.T, src string) *table.Library {
	t.Helper()
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	var b strings.Builder
	if err := RunProgram(prog, lib, log.New(&b)); err != nil {
		t.Fatalf("RunProgram: %v\nlog:\n%s", err, b.String())
	}
	return lib
}

const dsoSeed = "data people;\n  input id age dept $;\n  datalines;\n1 25 A\n2 40 B\n3 33 A\n4 19 C\n5 55 B\n;\nrun;\n"

func TestSetOptionsWhereKeepRename(t *testing.T) {
	lib := runSrc(t, dsoSeed+
		"data adults;\n  set people(where=(age >= 30) keep=id age dept rename=(age=years));\nrun;")
	ds, ok := lib.Get("adults")
	if !ok {
		t.Fatal("ADULTS not created")
	}
	if ds.NObs() != 3 {
		t.Errorf("NObs = %d, want 3 (ages 40,33,55)", ds.NObs())
	}
	if !ds.HasColumn("years") || ds.HasColumn("age") {
		t.Errorf("rename age->years failed; columns=%v", ds.ColumnNames())
	}
	if !ds.HasColumn("id") || !ds.HasColumn("dept") {
		t.Errorf("keep dropped wanted columns; columns=%v", ds.ColumnNames())
	}
	if got := ds.Get(ds.Rows[0], "years"); got.Num != 40 {
		t.Errorf("row0 years = %v, want 40", got.Display())
	}
}

func TestDataOutputDrop(t *testing.T) {
	lib := runSrc(t, dsoSeed+"data slim(drop=dept);\n  set people;\nrun;")
	ds, _ := lib.Get("slim")
	if ds.HasColumn("dept") {
		t.Errorf("drop=dept failed; columns=%v", ds.ColumnNames())
	}
	if ds.NObs() != 5 {
		t.Errorf("NObs = %d, want 5", ds.NObs())
	}
}

func TestMergeOptionsRename(t *testing.T) {
	src := "data a;\n input id x;\n datalines;\n1 10\n2 20\n3 30\n;\nrun;\n" +
		"data b;\n input id y;\n datalines;\n2 200\n3 300\n4 400\n;\nrun;\n" +
		"data m;\n merge a(in=ina) b(in=inb rename=(y=yval));\n by id;\n if ina and inb;\nrun;"
	lib := runSrc(t, src)
	ds, _ := lib.Get("m")
	if ds.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2 (matched id 2,3)", ds.NObs())
	}
	if !ds.HasColumn("yval") || ds.HasColumn("y") {
		t.Errorf("rename y->yval failed; columns=%v", ds.ColumnNames())
	}
	// in= flags are PDV-only temporaries and are not written to the output.
	if ds.HasColumn("ina") || ds.HasColumn("inb") {
		t.Errorf("in= flags should not be output columns; columns=%v", ds.ColumnNames())
	}
}

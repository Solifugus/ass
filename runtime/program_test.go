package runtime

import (
	"strings"
	"testing"

	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/table"
)

func TestRunProgramDataThenUnknownProc(t *testing.T) {
	// PROC TABULATE is not yet implemented, so it should be skipped with a note.
	src := "data people;\n  input name $ age;\n  datalines;\nJohn 25\nJane 30\n;\nrun;\n" +
		"proc tabulate data=people; run;"
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	var b strings.Builder
	if err := RunProgram(prog, lib, log.New(&b)); err != nil {
		t.Fatalf("RunProgram error: %v", err)
	}
	// The DATA step ran...
	ds, ok := lib.Get("people")
	if !ok || ds.NObs() != 2 {
		t.Fatalf("PEOPLE not built correctly: ok=%v", ok)
	}
	// ...and the unregistered PROC logged a NOTE rather than crashing.
	out := b.String()
	if !strings.Contains(out, "The data set WORK.PEOPLE has 2 observations") {
		t.Errorf("missing dataset note; log:\n%s", out)
	}
	if !strings.Contains(out, "PROC TABULATE is not supported") {
		t.Errorf("unknown proc should log not-supported note; log:\n%s", out)
	}
}

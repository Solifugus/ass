package runtime

import (
	"strings"
	"testing"

	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/table"
)

// runListing runs src and returns the captured procedure listing (LST) text.
func runListing(t *testing.T, src string) string {
	t.Helper()
	prog := parser.New(src).ParseProgram()
	var logBuf, lstBuf strings.Builder
	if err := RunProgram(prog, table.NewLibrary(), log.NewWith(&logBuf, &lstBuf)); err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
	return lstBuf.String()
}

func TestTitleAppearsAndPersists(t *testing.T) {
	// A title set before the first PROC should appear above both PROCs' output —
	// titles persist across steps until changed.
	out := runListing(t, `
title "Sales";
data t; input x; datalines;
1
2
;
run;
proc print data=t; run;
proc means data=t; var x; run;`)

	if n := strings.Count(out, "Sales"); n != 2 {
		t.Errorf("title appeared %d times, want 2 (once per PROC); output:\n%s", n, out)
	}
}

func TestTitleClears(t *testing.T) {
	// title; before the second PROC clears it, so only the first PROC shows it.
	out := runListing(t, `
title "Sales";
data t; input x; datalines;
1
;
run;
proc print data=t; run;
title;
proc print data=t; run;`)

	if n := strings.Count(out, "Sales"); n != 1 {
		t.Errorf("title appeared %d times, want 1 (cleared before 2nd PROC); output:\n%s", n, out)
	}
}

// TestRegHeaderHTMLRich confirms PROC REG emits its model summary and estimates
// as one rich block under a sink.
func TestRegHeaderHTMLRich(t *testing.T) {
	src := `
data t; input y x; datalines;
1 1
2 2
3 3
4 5
;
run;
proc reg data=t; model y = x; run;`
	prog := parser.New(src).ParseProgram()
	var htmlOut string
	logger := log.NewSink(func(ev log.Event) {
		if ev.Kind == "table" && ev.HTML != "" {
			htmlOut += ev.HTML
		}
	})
	if err := RunProgram(prog, table.NewLibrary(), logger); err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
	for _, want := range []string{"Dependent variable:", "R&sup2;", "Parameter Estimates", "<table"} {
		if !strings.Contains(htmlOut, want) {
			t.Errorf("REG rich output missing %q; got:\n%s", want, htmlOut)
		}
	}
}

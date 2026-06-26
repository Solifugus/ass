package proc_test

import (
	"testing"

	"github.com/solifugus/ass/table"
)

// TestFormatCntlout defines formats and writes them to a control dataset with
// CNTLOUT=, checking the standard column structure and the low/high/other flags.
func TestFormatCntlout(t *testing.T) {
	lib := runSrc(t, `proc format;
  value agegrp low-12='Child' 13-19='Teen' 20-high='Adult' other='?';
  value $reg 'E'='East' 'W'='West';
run;
proc format cntlout=fmts;
run;`)

	fmts, ok := lib.Get("fmts")
	if !ok {
		t.Fatal("CNTLOUT= did not create the control dataset")
	}
	if fmts.NObs() != 6 { // 4 agegrp ranges + 2 $reg ranges
		t.Fatalf("control dataset NObs = %d, want 6", fmts.NObs())
	}
	find := func(label string) table.Row {
		for _, r := range fmts.Rows {
			if fmts.Get(r, "label").Str == label {
				return r
			}
		}
		return nil
	}
	// low-12 'Child': START blank, END 12, HLO L, TYPE N.
	child := find("Child")
	if child == nil {
		t.Fatal("no 'Child' row")
	}
	if got := fmts.Get(child, "start").Str; got != "" {
		t.Errorf("Child START = %q, want empty", got)
	}
	if got := fmts.Get(child, "end").Str; got != "12" {
		t.Errorf("Child END = %q, want 12", got)
	}
	if got := fmts.Get(child, "hlo").Str; got != "L" {
		t.Errorf("Child HLO = %q, want L", got)
	}
	if got := fmts.Get(child, "type").Str; got != "N" {
		t.Errorf("Child TYPE = %q, want N", got)
	}
	// 20-high 'Adult': HLO H, START 20.
	adult := find("Adult")
	if got := fmts.Get(adult, "hlo").Str; got != "H" {
		t.Errorf("Adult HLO = %q, want H", got)
	}
	// other '?': HLO O.
	if got := fmts.Get(find("?"), "hlo").Str; got != "O" {
		t.Errorf("other HLO = %q, want O", got)
	}
	// character $reg 'East': TYPE C, START/END E.
	east := find("East")
	if got := fmts.Get(east, "type").Str; got != "C" {
		t.Errorf("East TYPE = %q, want C", got)
	}
	if got := fmts.Get(east, "start").Str; got != "E" {
		t.Errorf("East START = %q, want E", got)
	}
}

// TestFormatCntlin builds a control dataset in a DATA step, rebuilds formats from
// it with CNTLIN= (no VALUE statement), and confirms the rebuilt formats apply.
func TestFormatCntlin(t *testing.T) {
	lib := runSrc(t, `data fmtctl;
  length fmtname $8 start $8 end $8 label $10 type $1 hlo $3;
  fmtname='AGEGRP'; start='';   end='12'; label='Child';    type='N'; hlo='L'; output;
  fmtname='AGEGRP'; start='13'; end='19'; label='Teen';     type='N'; hlo='';  output;
  fmtname='AGEGRP'; start='20'; end='';   label='Adult';    type='N'; hlo='H'; output;
  fmtname='GRADE';  start='A';  end='A';  label='Excellent';type='C'; hlo='';  output;
run;
proc format cntlin=fmtctl;
run;
data t;
  input age g $;
  ageband = put(age, agegrp.);
  desc    = put(g, $grade.);
  datalines;
8 A
16 B
30 A
;
run;`)

	out, ok := lib.Get("t")
	if !ok {
		t.Fatal("CNTLIN= step did not produce t")
	}
	wantBand := []string{"Child", "Teen", "Adult"}
	wantDesc := []string{"Excellent", "B", "Excellent"} // B unmatched -> default value
	for i, r := range out.Rows {
		if got := out.Get(r, "ageband").Str; got != wantBand[i] {
			t.Errorf("row %d ageband = %q, want %q", i, got, wantBand[i])
		}
		if got := out.Get(r, "desc").Str; got != wantDesc[i] {
			t.Errorf("row %d desc = %q, want %q", i, got, wantDesc[i])
		}
	}
}

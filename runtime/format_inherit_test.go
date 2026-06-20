package runtime

import (
	"testing"

	"github.com/solifugus/ass/table"
)

// colFormat returns the display format of a named column in a dataset (or "" if
// the column is absent).
func colFormat(ds *table.Dataset, name string) string {
	for _, c := range ds.Columns {
		if c.Name == name {
			return c.Format
		}
	}
	return ""
}

// TestFormatInheritedThroughSet verifies that a variable keeps its source
// dataset's format when copied via SET, and that an explicit FORMAT statement in
// the reading step overrides it (SAS attribute inheritance).
func TestFormatInheritedThroughSet(t *testing.T) {
	src := `
data a;
  input x;
  format x date9.;
  datalines;
21929
;
run;

data inherit;
  set a;
run;

data override;
  set a;
  format x mmddyy10.;
run;
`
	lib := runProgram(t, src)

	inherit, ok := lib.Get("inherit")
	if !ok {
		t.Fatal("dataset INHERIT not created")
	}
	if got := colFormat(inherit, "x"); got != "date9" {
		t.Errorf("inherited format = %q, want date9", got)
	}

	override, ok := lib.Get("override")
	if !ok {
		t.Fatal("dataset OVERRIDE not created")
	}
	if got := colFormat(override, "x"); got != "mmddyy10" {
		t.Errorf("overridden format = %q, want mmddyy10", got)
	}
}

// TestFormatInheritedThroughMerge verifies attribute inheritance also works
// across a match-merge, first source winning for a shared variable.
func TestFormatInheritedThroughMerge(t *testing.T) {
	src := `
data left;
  input id amount;
  format amount dollar10.2;
  datalines;
1 100
2 200
;
run;

data right;
  input id hired;
  format hired date9.;
  datalines;
1 21929
2 22314
;
run;

data merged;
  merge left right;
  by id;
run;
`
	lib := runProgram(t, src)
	merged, ok := lib.Get("merged")
	if !ok {
		t.Fatal("dataset MERGED not created")
	}
	if got := colFormat(merged, "amount"); got != "dollar10.2" {
		t.Errorf("amount format = %q, want dollar10.2", got)
	}
	if got := colFormat(merged, "hired"); got != "date9" {
		t.Errorf("hired format = %q, want date9", got)
	}
}

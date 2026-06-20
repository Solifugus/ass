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

// colLabel returns the descriptive label of a named column (or "" if absent).
func colLabel(ds *table.Dataset, name string) string {
	for _, c := range ds.Columns {
		if c.Name == name {
			return c.Label
		}
	}
	return ""
}

// TestLabelStatementSetsLabel verifies a DATA step LABEL statement attaches a
// descriptive label to the variable's output column.
func TestLabelStatementSetsLabel(t *testing.T) {
	src := `
data t;
  input age;
  label age = "Age in Years";
  datalines;
25
;
run;
`
	lib := runProgram(t, src)
	ds, ok := lib.Get("t")
	if !ok {
		t.Fatal("dataset T not created")
	}
	if got := colLabel(ds, "age"); got != "Age in Years" {
		t.Errorf("age label = %q, want %q", got, "Age in Years")
	}
}

// TestLabelAsVariableName guards the parser disambiguation: `label = "..."` is an
// assignment to a variable named label, not a LABEL statement (label is not a
// reserved word in SAS).
func TestLabelAsVariableName(t *testing.T) {
	src := `
data t;
  label = "hi";
  output;
run;
`
	lib := runProgram(t, src)
	ds, ok := lib.Get("t")
	if !ok {
		t.Fatal("dataset T not created")
	}
	if ds.NObs() != 1 {
		t.Fatalf("nobs = %d, want 1", ds.NObs())
	}
	v := ds.Get(ds.Rows[0], "label")
	if v.Kind != table.Character || v.Str != "hi" {
		t.Errorf("label var = %q (kind %v), want \"hi\" char", v.Str, v.Kind)
	}
}

// TestLabelInheritedThroughSet verifies a label set in one step carries to a
// downstream step via SET, and an explicit LABEL there overrides it.
func TestLabelInheritedThroughSet(t *testing.T) {
	src := `
data a;
  input x;
  label x = "Original";
  datalines;
1
;
run;

data inherit;
  set a;
run;

data override;
  set a;
  label x = "New";
run;
`
	lib := runProgram(t, src)
	inherit, _ := lib.Get("inherit")
	if got := colLabel(inherit, "x"); got != "Original" {
		t.Errorf("inherited label = %q, want Original", got)
	}
	override, _ := lib.Get("override")
	if got := colLabel(override, "x"); got != "New" {
		t.Errorf("overridden label = %q, want New", got)
	}
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

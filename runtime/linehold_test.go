package runtime

import "testing"

// TestInputDoubleTrailingAt reads several observations from one physical line
// with `input x @@;` (line held across iterations).
func TestInputDoubleTrailingAt(t *testing.T) {
	src := "data nums;\n  input x @@;\n  datalines;\n1 2 3 4 5\n;\nrun;"
	lib := runStep(t, src)
	ds, ok := lib.Get("nums")
	if !ok {
		t.Fatal("NUMS not created")
	}
	if ds.NObs() != 5 {
		t.Fatalf("NObs = %d, want 5", ds.NObs())
	}
	if got := names(ds, "x"); !eqStr(got, []string{"1", "2", "3", "4", "5"}) {
		t.Errorf("x = %v, want [1 2 3 4 5]", got)
	}
}

// TestInputDoubleTrailingAtAcrossLines confirms `@@` spans physical lines: it
// keeps reading tokens until they are exhausted, regardless of line breaks.
func TestInputDoubleTrailingAtAcrossLines(t *testing.T) {
	src := "data pairs;\n  input a b @@;\n  datalines;\n1 2 3 4\n5 6\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("pairs")
	if ds.NObs() != 3 {
		t.Fatalf("NObs = %d, want 3", ds.NObs())
	}
	if got := names(ds, "a"); !eqStr(got, []string{"1", "3", "5"}) {
		t.Errorf("a = %v, want [1 3 5]", got)
	}
	if got := names(ds, "b"); !eqStr(got, []string{"2", "4", "6"}) {
		t.Errorf("b = %v, want [2 4 6]", got)
	}
}

// TestInputSingleTrailingAt holds the line within one iteration so a value can be
// read, tested, and the rest of the line read conditionally — the classic single
// trailing-`@` idiom. Each physical line is one observation.
func TestInputSingleTrailingAt(t *testing.T) {
	src := "data t;\n  input type $ @;\n  if type='A' then input x;\n  else input y;\n  datalines;\nA 10\nB 20\nA 30\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("t")
	if ds.NObs() != 3 {
		t.Fatalf("NObs = %d, want 3", ds.NObs())
	}
	if got := names(ds, "type"); !eqStr(got, []string{"A", "B", "A"}) {
		t.Errorf("type = %v, want [A B A]", got)
	}
	// Row 0 (A): x=10, y missing. Row 1 (B): y=20, x missing. Row 2 (A): x=30.
	if got := names(ds, "x"); !eqStr(got, []string{"10", ".", "30"}) {
		t.Errorf("x = %v, want [10 . 30]", got)
	}
	if got := names(ds, "y"); !eqStr(got, []string{".", "20", "."}) {
		t.Errorf("y = %v, want [. 20 .]", got)
	}
}

// TestInputNoTrailingAtUnchanged guards the default (no line-hold) path: one
// observation per record, exactly as before.
func TestInputNoTrailingAtUnchanged(t *testing.T) {
	src := "data p;\n  input name $ age;\n  datalines;\nJohn 25\nJane 30\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("p")
	if ds.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2", ds.NObs())
	}
	if got := names(ds, "name"); !eqStr(got, []string{"John", "Jane"}) {
		t.Errorf("name = %v, want [John Jane]", got)
	}
}

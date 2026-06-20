package runtime

import (
	"strings"
	"testing"
)

// TestPutSingleTrailingAt: a single trailing `@` holds the output line within the
// iteration so a later PUT continues it; the line is written at the iteration
// boundary. One line per observation, each carrying both values.
func TestPutSingleTrailingAt(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/out.txt"
	src := "data _null_;\n  input name $ age;\n  file \"" + path + "\";\n" +
		"  put name @;\n  put age;\n  datalines;\nAmy 30\nBob 40\n;\nrun;"
	runStep(t, src)
	got := strings.Split(strings.TrimRight(readFile(t, path), "\n"), "\n")
	want := []string{"Amy 30", "Bob 40"}
	if !eqStr(got, want) {
		t.Errorf("output = %v, want %v", got, want)
	}
}

// TestPutDoubleTrailingAt: a trailing `@@` holds the output line across
// iterations, accumulating values from several observations onto one physical
// line, released at end of step.
func TestPutDoubleTrailingAt(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/out.txt"
	src := "data _null_;\n  input x;\n  file \"" + path + "\";\n" +
		"  put x @@;\n  datalines;\n1\n2\n3\n;\nrun;"
	runStep(t, src)
	got := strings.Split(strings.TrimRight(readFile(t, path), "\n"), "\n")
	want := []string{"1 2 3"}
	if !eqStr(got, want) {
		t.Errorf("output = %v, want %v", got, want)
	}
}

// TestPutColumnTrailingAt: a column/pointer PUT held by `@` is continued by a
// later column/pointer PUT that overlays its values at their absolute columns on
// the same physical line.
func TestPutColumnTrailingAt(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/out.txt"
	src := "data _null_;\n  input name $ age;\n  file \"" + path + "\";\n" +
		"  put name $ 1-10 @;\n  put age 12-14;\n  datalines;\nAmy 30\n;\nrun;"
	runStep(t, src)
	got := strings.Split(strings.TrimRight(readFile(t, path), "\n"), "\n")
	// name in cols 1-3, age right-justified in cols 13-14.
	want := []string{"Amy" + strings.Repeat(" ", 9) + "30"}
	if !eqStr(got, want) {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestPutNoTrailingAtUnchanged guards the default path: without a hold each PUT
// writes its own line.
func TestPutNoTrailingAtUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/out.txt"
	src := "data _null_;\n  input name $ age;\n  file \"" + path + "\";\n" +
		"  put name;\n  put age;\n  datalines;\nAmy 30\n;\nrun;"
	runStep(t, src)
	got := strings.Split(strings.TrimRight(readFile(t, path), "\n"), "\n")
	want := []string{"Amy", "30"}
	if !eqStr(got, want) {
		t.Errorf("output = %v, want %v", got, want)
	}
}

// TestPutNamedOutput: `put id= name=;` renders each value as name=value.
func TestPutNamedOutput(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/named.txt"
	src := "data _null_;\n  input id name $;\n  file \"" + path + "\";\n" +
		"  put id= name=;\n  datalines;\n1 Amy\n2 Bob\n;\nrun;"
	runStep(t, src)
	got := strings.Split(strings.TrimRight(readFile(t, path), "\n"), "\n")
	want := []string{"id=1 name=Amy", "id=2 name=Bob"}
	if !eqStr(got, want) {
		t.Errorf("output = %v, want %v", got, want)
	}
}

// TestPutAllVars: `put _all_;` writes every PDV variable (including the
// automatic _n_/_error_) as name=value. We assert the user variables appear.
func TestPutAllVars(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/all.txt"
	src := "data _null_;\n  input id name $;\n  file \"" + path + "\";\n" +
		"  put _all_;\n  datalines;\n7 Cy\n;\nrun;"
	runStep(t, src)
	got := readFile(t, path)
	for _, want := range []string{"id=7", "name=Cy", "_n_=1"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q missing %q", got, want)
		}
	}
}

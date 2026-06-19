package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/table"
)

// runProg runs every DATA step in the program (in order) against one library and
// returns it. Non-DATA steps are ignored. This lets a test seed an input dataset
// and then run a writer step that consumes it.
func runProg(t *testing.T, src string) *table.Library {
	t.Helper()
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	for _, step := range prog.Steps {
		ds, ok := step.(*ast.DataStep)
		if !ok {
			continue
		}
		if err := RunDataStep(ds, lib, nil); err != nil {
			t.Fatalf("RunDataStep error: %v", err)
		}
	}
	return lib
}

// readFile returns a written output file's contents, failing the test on error.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestFilePutDSD(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.csv")
	lib := runProg(t, fmt.Sprintf(`data src;
  name = "Smith, John"; age = 25; city = "Boston"; output;
  name = "Mary"; age = .; city = "New York"; output;
run;
data _null_;
  set src;
  file "%s" dsd;
  put name age city;
run;`, out))

	got := readFile(t, out)
	// Only values containing the delimiter are quoted: "Smith, John" is quoted,
	// "New York" (a space, no comma) is not.
	want := "\"Smith, John\",25,Boston\nMary,.,New York\n"
	if got != want {
		t.Errorf("DSD output:\n got %q\nwant %q", got, want)
	}
	if _, ok := lib.Get("_null_"); ok { // _null_ must not create a dataset
		t.Error("data _null_ created a dataset")
	}
}

func TestFilePutColumnOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.txt")
	runProg(t, fmt.Sprintf(`data src;
  name = "Mary Ann"; age = 42; output;
  name = "Bob"; age = 7; output;
run;
data _null_;
  set src;
  file "%s";
  put name $ 1-10 age 11-13;
run;`, out))

	got := readFile(t, out)
	// NAME left-justified in cols 1-10, AGE right-justified in cols 11-13.
	// Trailing blanks are trimmed from each line.
	want := "Mary Ann   42\nBob         7\n"
	if got != want {
		t.Errorf("column output:\n got %q\nwant %q", got, want)
	}
}

func TestFilePutPointerOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.txt")
	runProg(t, fmt.Sprintf(`data src;
  id = 7; label = "X"; output;
run;
data _null_;
  set src;
  file "%s";
  put @5 label $ @10 id 3.;
run;`, out))

	got := readFile(t, out)
	// label "X" placed at col 5; id formatted (3.) "7" placed starting at col 10.
	want := "    X    7\n"
	if got != want {
		t.Errorf("pointer output:\n got %q\nwant %q", got, want)
	}
}

func TestFilePutListDefault(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.txt")
	runProg(t, fmt.Sprintf(`data src;
  a = 1; b = 2; output;
  a = 3; b = 4; output;
run;
data _null_;
  set src;
  file "%s";
  put a b;
run;`, out))

	got := readFile(t, out)
	want := "1 2\n3 4\n"
	if got != want {
		t.Errorf("list output:\n got %q\nwant %q", got, want)
	}
}

func TestFilePutDelimiterAndLiteral(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.txt")
	runProg(t, fmt.Sprintf(`data src;
  x = 1; y = 2; output;
run;
data _null_;
  set src;
  file "%s" dlm="|";
  put "row" x y;
run;`, out))

	got := readFile(t, out)
	want := "row|1|2\n"
	if got != want {
		t.Errorf("delimited output:\n got %q\nwant %q", got, want)
	}
}

func TestFilePutInlineFormat(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.txt")
	runProg(t, fmt.Sprintf(`data src;
  amt = 1234.5; output;
run;
data _null_;
  set src;
  file "%s";
  put amt dollar10.2;
run;`, out))

	got := readFile(t, out)
	want := "$1,234.50\n"
	if got != want {
		t.Errorf("formatted output:\n got %q\nwant %q", got, want)
	}
}

func TestPutRoundTrip(t *testing.T) {
	// Write a CSV with PUT/DSD, then read it back with INFILE/DSD and verify the
	// values survive the round trip (including the embedded comma).
	out := filepath.Join(t.TempDir(), "rt.csv")
	back := runProg(t, fmt.Sprintf(`data src;
  name = "Doe, Jane"; n = 7; output;
run;
data _null_;
  set src;
  file "%s" dsd;
  put name n;
run;
data back;
  infile "%s" dsd;
  input name $ n;
run;`, out, out))

	ds, ok := back.Get("back")
	if !ok {
		t.Fatal("dataset back not created")
	}
	if ds.NObs() != 1 {
		t.Fatalf("NObs = %d, want 1", ds.NObs())
	}
	if got := ds.Get(ds.Rows[0], "name"); got.Str != "Doe, Jane" {
		t.Errorf("name = %q, want %q", got.Str, "Doe, Jane")
	}
	if got := ds.Get(ds.Rows[0], "n"); got.Num != 7 {
		t.Errorf("n = %v, want 7", got.Num)
	}
}

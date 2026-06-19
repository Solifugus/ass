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

// writeTemp writes content to a temp file and returns its path.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestInfileDSDQuotedAndMissing(t *testing.T) {
	path := writeTemp(t, "people.csv",
		"name,age,city\n\"Smith, John\",25,Boston\nMary,30,\"New York\"\nTim,,Chicago\n")
	src := fmt.Sprintf(`data people;
  infile "%s" dsd firstobs=2;
  input name $ age city $;
run;`, path)
	lib := runStep(t, src)

	ds, ok := lib.Get("people")
	if !ok {
		t.Fatal("dataset people not created")
	}
	if ds.NObs() != 3 {
		t.Fatalf("NObs = %d, want 3", ds.NObs())
	}
	// Row 0: embedded comma preserved inside quotes.
	if got := ds.Get(ds.Rows[0], "name"); got.Str != "Smith, John" {
		t.Errorf("row0 name = %q, want %q", got.Str, "Smith, John")
	}
	if got := ds.Get(ds.Rows[0], "age"); got.Num != 25 {
		t.Errorf("row0 age = %v, want 25", got.Num)
	}
	// Row 1: quoted field with a space.
	if got := ds.Get(ds.Rows[1], "city"); got.Str != "New York" {
		t.Errorf("row1 city = %q, want %q", got.Str, "New York")
	}
	// Row 2: empty field between two delimiters -> numeric missing.
	if got := ds.Get(ds.Rows[2], "age"); !got.IsMissing() {
		t.Errorf("row2 age = %v, want missing", got)
	}
	if got := ds.Get(ds.Rows[2], "name"); got.Str != "Tim" {
		t.Errorf("row2 name = %q, want %q", got.Str, "Tim")
	}
}

func TestInfileWhitespaceFirstobsObs(t *testing.T) {
	path := writeTemp(t, "nums.txt", "id val\n1 10\n2 20\n3 30\n4 40\n")
	src := fmt.Sprintf(`data subset;
  infile "%s" firstobs=2 obs=3;
  input id val;
run;`, path)
	lib := runStep(t, src)

	ds, _ := lib.Get("subset")
	if ds.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2 (lines 2..3)", ds.NObs())
	}
	want := []struct{ id, val float64 }{{1, 10}, {2, 20}}
	for i, w := range want {
		if got := ds.Get(ds.Rows[i], "id"); got.Num != w.id {
			t.Errorf("row%d id = %v, want %v", i, got.Num, w.id)
		}
		if got := ds.Get(ds.Rows[i], "val"); got.Num != w.val {
			t.Errorf("row%d val = %v, want %v", i, got.Num, w.val)
		}
	}
}

func TestInfileDelimiterNoDSDCollapses(t *testing.T) {
	// DLM without DSD: consecutive delimiters collapse to a single one.
	path := writeTemp(t, "p.txt", "a;;b;c\n")
	src := fmt.Sprintf(`data t;
  infile "%s" dlm=";";
  input x $ y $ z $;
run;`, path)
	lib := runStep(t, src)
	ds, _ := lib.Get("t")
	if ds.NObs() != 1 {
		t.Fatalf("NObs = %d, want 1", ds.NObs())
	}
	got := []string{
		ds.Get(ds.Rows[0], "x").Str,
		ds.Get(ds.Rows[0], "y").Str,
		ds.Get(ds.Rows[0], "z").Str,
	}
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("field %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestInfileMissingFileErrors(t *testing.T) {
	src := `data t;
  infile "/no/such/file.csv";
  input x;
run;`
	prog := parser.New(src).ParseProgram()
	ds, ok := prog.Steps[0].(*ast.DataStep)
	if !ok {
		t.Fatalf("first step is %T, want *ast.DataStep", prog.Steps[0])
	}
	lib := table.NewLibrary()
	if err := RunDataStep(ds, lib, nil); err == nil {
		t.Fatal("expected an error for a missing infile path, got nil")
	}
}

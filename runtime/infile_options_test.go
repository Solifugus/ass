package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/solifugus/ass/table"
)

// TestInfileEnd verifies INFILE END= sets a flag on the last record (used here to
// emit a single summary row) and that the flag variable is not written out.
func TestInfileEnd(t *testing.T) {
	in := filepath.Join(t.TempDir(), "people.txt")
	if err := os.WriteFile(in, []byte("Alice 30\nBob 25\nCara 41\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := runProg(t, fmt.Sprintf(`data summary;
  infile "%s" end=last;
  input name $ age;
  total + age;
  if last then output;
run;`, in))

	ds, ok := lib.Get("summary")
	if !ok {
		t.Fatal("summary not created")
	}
	if ds.NObs() != 1 {
		t.Fatalf("NObs = %d, want 1 (only the last record outputs)", ds.NObs())
	}
	row := ds.Rows[0]
	if got := ds.Get(row, "total"); got.Num != 96 {
		t.Errorf("total = %v, want 96", got.Display())
	}
	if got := ds.Get(row, "name"); got.Str != "Cara" {
		t.Errorf("name = %q, want Cara", got.Str)
	}
	// END= flag is temporary, not a dataset column.
	for _, c := range ds.Columns {
		if c.Name == "last" {
			t.Error("END= variable 'last' leaked into the output dataset")
		}
	}
}

// TestInfilePadLrecl verifies PAD blank-pads short records to LRECL so fixed
// column input reads missing for absent columns instead of erroring.
func TestInfilePadLrecl(t *testing.T) {
	in := filepath.Join(t.TempDir(), "cols.txt")
	if err := os.WriteFile(in, []byte("AB123\nCD\nEF45\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := runProg(t, fmt.Sprintf(`data t;
  infile "%s" pad lrecl=5;
  input code $ 1-2 num 3-5;
run;`, in))

	ds, _ := lib.Get("t")
	wantNumCol := []struct {
		code string
		num  table.Value
	}{
		{"AB", table.Num(123)},
		{"CD", table.MissingNum()},
		{"EF", table.Num(45)},
	}
	if ds.NObs() != 3 {
		t.Fatalf("NObs = %d, want 3", ds.NObs())
	}
	for i, w := range wantNumCol {
		r := ds.Rows[i]
		if got := ds.Get(r, "code"); got.Str != w.code {
			t.Errorf("row %d code = %q, want %q", i, got.Str, w.code)
		}
		got := ds.Get(r, "num")
		if w.num.IsMissing() {
			if !got.IsMissing() {
				t.Errorf("row %d num = %v, want missing", i, got.Display())
			}
		} else if got.Num != w.num.Num {
			t.Errorf("row %d num = %v, want %v", i, got.Display(), w.num.Num)
		}
	}
}

// TestFileMod verifies the FILE MOD option appends to an existing file.
func TestFileMod(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.txt")
	runProg(t, fmt.Sprintf(`data a; x=1; output; run;
data _null_; set a; file "%s"; put "first"; run;
data _null_; set a; file "%s" mod; put "second"; run;`, out, out))

	if got := readFile(t, out); got != "first\nsecond\n" {
		t.Errorf("mod append = %q, want \"first\\nsecond\\n\"", got)
	}
}

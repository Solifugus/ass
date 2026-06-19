package proc_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExportCSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")
	src := "data staff;\n  input name $ salary;\n  datalines;\nSmith 52000\nMary 48500\n;\nrun;\n" +
		"proc export data=staff outfile=\"" + path + "\" dbms=csv replace;\nrun;"
	runSrc(t, src)

	got := readTextFile(t, path)
	want := "name,salary\nSmith,52000\nMary,48500\n"
	if got != want {
		t.Errorf("export = %q, want %q", got, want)
	}
}

func TestExportQuotingAndMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")
	// Build a row with an embedded comma and a missing numeric via SET from
	// datalines is awkward; use DSD infile to seed the comma value.
	seed := filepath.Join(dir, "seed.csv")
	if err := os.WriteFile(seed, []byte("\"Smith, John\",52000\nMary,\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "data staff;\n  infile \"" + seed + "\" dsd;\n  input name $ salary;\nrun;\n" +
		"proc export data=staff outfile=\"" + path + "\" dbms=csv replace;\nrun;"
	runSrc(t, src)

	got := readTextFile(t, path)
	// "Smith, John" re-quoted (embedded comma); missing numeric -> empty field.
	want := "name,salary\n\"Smith, John\",52000\nMary,\n"
	if got != want {
		t.Errorf("export = %q, want %q", got, want)
	}
}

func TestImportCSVTypeSniffing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.csv")
	if err := os.WriteFile(path, []byte("name,salary\nSmith,52000\nMary,48500\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "proc import datafile=\"" + path + "\" out=staff dbms=csv replace;\n  getnames=yes;\nrun;"
	lib := runSrc(t, src)
	ds, ok := lib.Get("staff")
	if !ok {
		t.Fatal("dataset staff not created")
	}
	if got := ds.ColumnNames(); !eq(got, []string{"name", "salary"}) {
		t.Errorf("columns = %v, want [name salary]", got)
	}
	if ds.Columns[1].Kind.String() != "num" {
		t.Errorf("salary kind = %s, want num", ds.Columns[1].Kind)
	}
	if got := names(ds, "salary"); !eq(got, []string{"52000", "48500"}) {
		t.Errorf("salary = %v, want [52000 48500]", got)
	}
}

func TestImportGetnamesNo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.csv")
	if err := os.WriteFile(path, []byte("Smith,52000\nMary,48500\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "proc import datafile=\"" + path + "\" out=staff dbms=csv replace;\n  getnames=no;\nrun;"
	lib := runSrc(t, src)
	ds, _ := lib.Get("staff")
	if got := ds.ColumnNames(); !eq(got, []string{"VAR1", "VAR2"}) {
		t.Errorf("columns = %v, want [VAR1 VAR2]", got)
	}
	if got := names(ds, "VAR1"); !eq(got, []string{"Smith", "Mary"}) {
		t.Errorf("VAR1 = %v, want [Smith Mary]", got)
	}
}

func TestImportEmbeddedCommaAndMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.csv")
	if err := os.WriteFile(path, []byte("name,salary\n\"Smith, John\",52000\nMary,\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "proc import datafile=\"" + path + "\" out=staff dbms=csv replace;\nrun;"
	lib := runSrc(t, src)
	ds, _ := lib.Get("staff")
	if got := names(ds, "name"); !eq(got, []string{"Smith, John", "Mary"}) {
		t.Errorf("name = %v, want [\"Smith, John\" Mary]", got)
	}
	// Missing numeric prints as "." via Display().
	if got := names(ds, "salary"); !eq(got, []string{"52000", "."}) {
		t.Errorf("salary = %v, want [52000 .]", got)
	}
}

func TestImportExportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rt.csv")
	src := "data staff;\n  input name $ dept $ salary;\n  datalines;\nSmith Sales 52000\nMary Admin 48500\n;\nrun;\n" +
		"proc export data=staff outfile=\"" + path + "\" dbms=csv replace;\nrun;\n" +
		"proc import datafile=\"" + path + "\" out=back dbms=csv replace;\nrun;"
	lib := runSrc(t, src)
	ds, _ := lib.Get("back")
	if got := ds.ColumnNames(); !eq(got, []string{"name", "dept", "salary"}) {
		t.Errorf("columns = %v", got)
	}
	if got := names(ds, "salary"); !eq(got, []string{"52000", "48500"}) {
		t.Errorf("salary = %v, want [52000 48500]", got)
	}
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

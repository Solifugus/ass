package xlsx

import (
	"path/filepath"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.xlsx")
	rows := [][]string{
		{"region", "amount"},
		{"North", "100"},
		{"South", "250"},
		{"East,West", "75"}, // comma + would-be CSV trap; xlsx keeps it intact
	}
	// All non-header cells numeric only in column 1.
	numericCol := func(rowIdx, col int) bool { return rowIdx > 0 && col == 1 }
	if err := Write(path, rows, numericCol); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != len(rows) {
		t.Fatalf("rows = %d, want %d", len(got), len(rows))
	}
	for i := range rows {
		if len(got[i]) != len(rows[i]) {
			t.Fatalf("row %d width = %d, want %d (%v)", i, len(got[i]), len(rows[i]), got[i])
		}
		for j := range rows[i] {
			if got[i][j] != rows[i][j] {
				t.Errorf("cell [%d][%d] = %q, want %q", i, j, got[i][j], rows[i][j])
			}
		}
	}
}

func TestColRef(t *testing.T) {
	cases := map[int]string{0: "A", 1: "B", 25: "Z", 26: "AA", 27: "AB"}
	for idx, want := range cases {
		if got := colRef(idx); got != want {
			t.Errorf("colRef(%d) = %q, want %q", idx, got, want)
		}
		if got := colIndex(want+"1", -1); got != idx {
			t.Errorf("colIndex(%q) = %d, want %d", want+"1", got, idx)
		}
	}
}

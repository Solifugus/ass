package proc

import (
	"testing"

	"github.com/solifugus/ass/table"
)

// TestInheritSourceFormats verifies a query result column picks up the format of
// the like-named source column (and that aggregates/renames, which have no source
// match, are left unformatted).
func TestInheritSourceFormats(t *testing.T) {
	lib := table.NewLibrary()
	src := table.NewDataset("", "sales")
	src.AddColumn(table.Column{Name: "region", Kind: table.Character, Format: "$reg."})
	src.AddColumn(table.Column{Name: "amt", Kind: table.Numeric, Format: "band."})
	if err := lib.Store("sales", src); err != nil {
		t.Fatalf("store: %v", err)
	}

	// A typical query result: region + amt carried through, plus an aggregate.
	res := table.NewDataset("", "_sql_result_")
	res.AddColumn(table.Column{Name: "region", Kind: table.Character})
	res.AddColumn(table.Column{Name: "amt", Kind: table.Numeric})
	res.AddColumn(table.Column{Name: "n", Kind: table.Numeric}) // count(*) — no source

	inheritSourceFormats(res, lib)

	if got := res.Columns[0].Format; got != "$reg." {
		t.Errorf("region format = %q, want $reg.", got)
	}
	if got := res.Columns[1].Format; got != "band." {
		t.Errorf("amt format = %q, want band.", got)
	}
	if got := res.Columns[2].Format; got != "" {
		t.Errorf("aggregate column format = %q, want empty", got)
	}
}

// TestInheritSourceFormatsPreservesExisting confirms a result column that already
// has a format keeps it (no clobbering).
func TestInheritSourceFormatsPreservesExisting(t *testing.T) {
	lib := table.NewLibrary()
	src := table.NewDataset("", "t")
	src.AddColumn(table.Column{Name: "x", Kind: table.Numeric, Format: "band."})
	if err := lib.Store("t", src); err != nil {
		t.Fatalf("store: %v", err)
	}
	res := table.NewDataset("", "_sql_result_")
	res.AddColumn(table.Column{Name: "x", Kind: table.Numeric, Format: "dollar8.2"})
	inheritSourceFormats(res, lib)
	if got := res.Columns[0].Format; got != "dollar8.2" {
		t.Errorf("x format = %q, want dollar8.2 (unchanged)", got)
	}
}

package proc

import (
	"testing"

	"github.com/solifugus/ass/table"
)

func samplePeople() *table.Dataset {
	ds := table.NewDataset("", "people")
	ds.AddColumn(table.Column{Name: "name", Kind: table.Character})
	ds.AddColumn(table.Column{Name: "age", Kind: table.Numeric})
	ds.AppendRow(table.Row{"name": table.Char("John"), "age": table.Num(25)})
	ds.AppendRow(table.Row{"name": table.Char("Jane"), "age": table.Num(30)})
	return ds
}

func TestRenderListing(t *testing.T) {
	got := renderListing(samplePeople(), printOptions{})
	want := "Obs  name  age\n\n" +
		"  1  John   25\n" +
		"  2  Jane   30\n"
	if got != want {
		t.Errorf("listing mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderListingNoobs(t *testing.T) {
	got := renderListing(samplePeople(), printOptions{noobs: true})
	want := "name  age\n\n" +
		"John   25\n" +
		"Jane   30\n"
	if got != want {
		t.Errorf("noobs listing mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderListingVarSelection(t *testing.T) {
	// Only age, and as the sole column.
	got := renderListing(samplePeople(), printOptions{vars: []string{"age"}})
	want := "Obs  age\n\n" +
		"  1   25\n" +
		"  2   30\n"
	if got != want {
		t.Errorf("var-selection listing mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderListingVarReorder(t *testing.T) {
	// age before name.
	got := renderListing(samplePeople(), printOptions{vars: []string{"age", "name"}})
	want := "Obs  age  name\n\n" +
		"  1   25  John\n" +
		"  2   30  Jane\n"
	if got != want {
		t.Errorf("var-reorder listing mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderListingLabel(t *testing.T) {
	ds := table.NewDataset("", "people")
	ds.AddColumn(table.Column{Name: "age", Kind: table.Numeric, Label: "Age in Years"})
	ds.AppendRow(table.Row{"age": table.Num(25)})
	// With label option, the header is the column label, wrapped at blanks so the
	// column is only as wide as the data (here the longest word "Years" = 5).
	got := renderListing(ds, printOptions{noobs: true, label: true})
	want := "  Age\n" +
		"   in\n" +
		"Years\n\n" +
		"   25\n"
	if got != want {
		t.Errorf("label listing mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	// Without label option, the header is the variable name.
	got = renderListing(ds, printOptions{noobs: true})
	want = "age\n\n 25\n"
	if got != want {
		t.Errorf("no-label listing mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderListingLabelStatementOverride(t *testing.T) {
	ds := table.NewDataset("", "people")
	ds.AddColumn(table.Column{Name: "age", Kind: table.Numeric, Label: "Stored Label"})
	ds.AppendRow(table.Row{"age": table.Num(25)})
	// A LABEL statement in the step overrides the variable's stored label.
	got := renderListing(ds, printOptions{noobs: true, label: true,
		labels: map[string]string{"age": "Step Label"}})
	want := " Step\n" +
		"Label\n\n" +
		"   25\n"
	if got != want {
		t.Errorf("label-override listing mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderListingLabelWrapBottomAligned(t *testing.T) {
	// Two columns with different header-line counts: a long wrapped label beside a
	// short one. The shorter header bottom-aligns to the last header row, and a
	// label that fits the data width stays on one line.
	ds := table.NewDataset("", "t")
	ds.AddColumn(table.Column{Name: "n", Kind: table.Numeric, Label: "Number of Visits"})
	ds.AddColumn(table.Column{Name: "city", Kind: table.Character, Label: "City"})
	ds.AppendRow(table.Row{"n": table.Num(3), "city": table.Char("Chicago")})

	got := renderListing(ds, printOptions{noobs: true, label: true})
	// "Number of Visits": longest word "Number" = 6 > data width 1 -> width 6,
	// wraps to ["Number","of","Visits"] (3 lines). "City": data "Chicago" = 7,
	// label fits -> 1 line, bottom-aligned to the third row.
	want := "Number\n" +
		"    of\n" +
		"Visits  City\n\n" +
		"     3  Chicago\n"
	if got != want {
		t.Errorf("wrap/bottom-align mismatch:\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestRenderListingMissingValue(t *testing.T) {
	ds := table.NewDataset("", "t")
	ds.AddColumn(table.Column{Name: "x", Kind: table.Numeric})
	ds.AppendRow(table.Row{"x": table.Num(5)})
	ds.AppendRow(table.Row{"x": table.MissingNum()})
	got := renderListing(ds, printOptions{noobs: true})
	want := "x\n\n5\n.\n"
	if got != want {
		t.Errorf("missing-value listing mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

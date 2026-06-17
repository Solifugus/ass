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
	// With label option, the header is the column label.
	got := renderListing(ds, printOptions{noobs: true, label: true})
	want := "Age in Years\n\n" +
		"          25\n"
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

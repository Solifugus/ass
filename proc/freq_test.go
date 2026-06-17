package proc

import (
	"strings"
	"testing"

	"github.com/solifugus/ass/table"
)

func petsDS() *table.Dataset {
	ds := table.NewDataset("", "pets")
	ds.AddColumn(table.Column{Name: "kind", Kind: table.Character})
	for _, k := range []string{"cat", "dog", "cat", "fish", "dog", "cat"} {
		ds.AppendRow(table.Row{"kind": table.Char(k)})
	}
	return ds
}

func TestFreqOneWay(t *testing.T) {
	res := buildFreqResult(petsDS(), "kind")
	if res.NObs() != 3 { // cat, dog, fish
		t.Fatalf("NObs = %d, want 3", res.NObs())
	}
	// Sorted alphabetically: cat, dog, fish.
	if got := res.Get(res.Rows[0], "kind"); got.Str != "cat" {
		t.Errorf("row0 = %q, want cat", got.Str)
	}
	if got := res.Get(res.Rows[0], "Frequency"); got.Num != 3 {
		t.Errorf("cat frequency = %v, want 3", got.Display())
	}
	// Cumulative on the last row is the total count and 100%.
	last := res.Rows[2]
	if got := res.Get(last, "CumFreq"); got.Num != 6 {
		t.Errorf("cumfreq = %v, want 6", got.Display())
	}
	if got := res.Get(last, "CumPercent"); got.Num != 100 {
		t.Errorf("cumpercent = %v, want 100", got.Display())
	}
}

func TestFreqExcludesMissing(t *testing.T) {
	ds := table.NewDataset("", "t")
	ds.AddColumn(table.Column{Name: "x", Kind: table.Character})
	ds.AppendRow(table.Row{"x": table.Char("a")})
	ds.AppendRow(table.Row{"x": table.MissingChar()})
	ds.AppendRow(table.Row{"x": table.Char("a")})
	res := buildFreqResult(ds, "x")
	if res.NObs() != 1 {
		t.Fatalf("NObs = %d, want 1 (missing excluded)", res.NObs())
	}
	if got := res.Get(res.Rows[0], "Percent"); got.Num != 100 {
		t.Errorf("percent = %v, want 100 (2 of 2 non-missing)", got.Display())
	}
}

func salesDS() *table.Dataset {
	ds := table.NewDataset("", "sales")
	ds.AddColumn(table.Column{Name: "region", Kind: table.Character})
	ds.AddColumn(table.Column{Name: "product", Kind: table.Character})
	rows := [][2]string{
		{"North", "A"}, {"North", "A"}, {"North", "B"}, {"North", "A"}, {"North", "B"},
		{"South", "A"}, {"South", "B"}, {"South", "B"}, {"South", "B"}, {"South", "B"},
	}
	for _, r := range rows {
		ds.AppendRow(table.Row{"region": table.Char(r[0]), "product": table.Char(r[1])})
	}
	return ds
}

func TestFreqTwoWayCrossTab(t *testing.T) {
	out := renderCrossTab(salesDS(), "region", "product")
	// Header and structure.
	for _, want := range []string{
		"Table of region by product",
		"Frequency", "Percent", "Row Pct", "Col Pct",
		"North", "South", "Total",
	} {
		if !contains(out, want) {
			t.Errorf("crosstab missing %q\n%s", want, out)
		}
	}
	// Spot-check computed cells: North/A freq 3, row pct 60.00, col pct 75.00;
	// North/B col pct 33.33; grand total 10.
	for _, want := range []string{"60.00", "75.00", "33.33", "66.67"} {
		if !contains(out, want) {
			t.Errorf("crosstab missing computed value %q\n%s", want, out)
		}
	}
	// Grand total line: column totals 4, 6 and grand 10.
	if !contains(out, "100.00") {
		t.Errorf("crosstab missing grand-total percent 100.00\n%s", out)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

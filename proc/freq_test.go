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

// dispFmt is the default (no user format) category formatter for FREQ tests.
func dispFmt(v table.Value) string { return v.Display() }

func TestFreqOneWay(t *testing.T) {
	res := buildFreqResult(petsDS(), "kind", dispFmt, false)
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
	res := buildFreqResult(ds, "x", dispFmt, false)
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
	out := renderCrossTab(salesDS(), "region", "product", dispFmt, dispFmt)
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

// TestFreqUserFormatGrouping verifies FREQ groups by the formatted category when
// a user VALUE format applies: low-29 -> "Young", 30-high -> "Older".
func TestFreqUserFormatGrouping(t *testing.T) {
	cat := table.NewFormatCatalog()
	cat.Define(&table.ValueFormat{
		Name: "agegrp",
		Ranges: []table.FormatRange{
			{NoLow: true, High: table.Num(29), Label: "Young"},
			{Low: table.Num(30), NoHigh: true, Label: "Older"},
		},
	})
	ds := table.NewDataset("", "people")
	ds.AddColumn(table.Column{Name: "age", Kind: table.Numeric, Format: "agegrp."})
	for _, a := range []float64{22, 25, 40, 55, 33} {
		ds.AppendRow(table.Row{"age": table.Num(a)})
	}
	fmtFn := freqFormatter(ds, cat, map[string]string{}, "age")
	res := buildFreqResult(ds, "age", fmtFn, true)
	if res.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2 (Young, Older)", res.NObs())
	}
	if got := res.Get(res.Rows[0], "age").Str; got != "Young" {
		t.Errorf("cat0 = %q, want Young", got)
	}
	if got := res.Get(res.Rows[0], "Frequency").Num; got != 2 {
		t.Errorf("Young freq = %v, want 2", got)
	}
	if got := res.Get(res.Rows[1], "age").Str; got != "Older" {
		t.Errorf("cat1 = %q, want Older", got)
	}
	if got := res.Get(res.Rows[1], "Frequency").Num; got != 3 {
		t.Errorf("Older freq = %v, want 3", got)
	}
}

// TestFreqNWayList verifies the n-way list-format table: distinct combinations
// with frequencies, ordered by underlying values.
func TestFreqNWayList(t *testing.T) {
	ds := table.NewDataset("", "d")
	ds.AddColumn(table.Column{Name: "a", Kind: table.Character})
	ds.AddColumn(table.Column{Name: "b", Kind: table.Character})
	add := func(a, b string) { ds.AppendRow(table.Row{"a": table.Char(a), "b": table.Char(b)}) }
	add("x", "1")
	add("x", "1")
	add("x", "2")
	add("y", "1")
	fmtFor := func(string) func(table.Value) string { return func(v table.Value) string { return v.Display() } }
	fmtdFor := func(string) bool { return false }
	res := buildFreqResultN(ds, []string{"a", "b"}, fmtFor, fmtdFor)
	if res.NObs() != 3 { // (x,1),(x,2),(y,1)
		t.Fatalf("NObs = %d, want 3", res.NObs())
	}
	// First combo (x,1) has frequency 2.
	if got := res.Get(res.Rows[0], "Frequency").Num; got != 2 {
		t.Errorf("(x,1) freq = %v, want 2", got)
	}
	if a, b := res.Get(res.Rows[0], "a").Str, res.Get(res.Rows[0], "b").Str; a != "x" || b != "1" {
		t.Errorf("combo0 = (%s,%s), want (x,1)", a, b)
	}
}

// TestChiSquareStat verifies the Pearson chi-square on a 2x2 table with a
// hand-computed statistic.
func TestChiSquareStat(t *testing.T) {
	ds := table.NewDataset("", "d")
	ds.AddColumn(table.Column{Name: "r", Kind: table.Character})
	ds.AddColumn(table.Column{Name: "c", Kind: table.Character})
	add := func(r, c string, n int) {
		for i := 0; i < n; i++ {
			ds.AppendRow(table.Row{"r": table.Char(r), "c": table.Char(c)})
		}
	}
	// Cells: r1c1=10, r1c2=20, r2c1=30, r2c2=40. Expected chi-square ~0.7937.
	add("r1", "c1", 10)
	add("r1", "c2", 20)
	add("r2", "c1", 30)
	add("r2", "c2", 40)
	disp := func(v table.Value) string { return v.Display() }
	stat, df, p := chiSquareStat(ds, "r", "c", disp, disp)
	if df != 1 {
		t.Errorf("df = %d, want 1", df)
	}
	if stat < 0.792 || stat > 0.795 {
		t.Errorf("chi-square = %.4f, want ~0.7937", stat)
	}
	if p < 0.37 || p > 0.38 { // pchisq(0.7937,1,lower=F) ~ 0.3729
		t.Errorf("p = %.4f, want ~0.373", p)
	}
}

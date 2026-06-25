package proc

import (
	"testing"

	"github.com/solifugus/ass/table"
)

func scoresDS() *table.Dataset {
	ds := table.NewDataset("", "scores")
	ds.AddColumn(table.Column{Name: "grp", Kind: table.Character})
	ds.AddColumn(table.Column{Name: "score", Kind: table.Numeric})
	add := func(g string, s float64) { ds.AppendRow(table.Row{"grp": table.Char(g), "score": table.Num(s)}) }
	add("a", 10)
	add("a", 20)
	add("a", 30)
	add("b", 100)
	add("b", 200)
	return ds
}

func TestMeansNoClass(t *testing.T) {
	res := buildMeansResult(scoresDS(), []string{"score"}, nil, defaultMeanStats(), -1, nil, nil)
	if res.NObs() != 1 {
		t.Fatalf("NObs = %d, want 1", res.NObs())
	}
	r := res.Rows[0]
	if got := res.Get(r, "N"); got.Num != 5 {
		t.Errorf("N = %v, want 5", got.Display())
	}
	if got := res.Get(r, "Mean"); got.Num != 72 {
		t.Errorf("Mean = %v, want 72", got.Display())
	}
	if got := res.Get(r, "Min"); got.Num != 10 {
		t.Errorf("Min = %v, want 10", got.Display())
	}
	if got := res.Get(r, "Max"); got.Num != 200 {
		t.Errorf("Max = %v, want 200", got.Display())
	}
}

func TestMeansWithClass(t *testing.T) {
	res := buildMeansResult(scoresDS(), []string{"score"}, []string{"grp"}, defaultMeanStats(), -1, nil, nil)
	if res.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2 (one per group)", res.NObs())
	}
	byGrp := map[string]table.Row{}
	for _, r := range res.Rows {
		byGrp[res.Get(r, "grp").Str] = r
	}
	if got := res.Get(byGrp["a"], "Mean"); got.Num != 20 {
		t.Errorf("group a Mean = %v, want 20", got.Display())
	}
	if got := res.Get(byGrp["a"], "StdDev"); got.Num != 10 {
		t.Errorf("group a StdDev = %v, want 10", got.Display())
	}
	if got := res.Get(byGrp["b"], "Mean"); got.Num != 150 {
		t.Errorf("group b Mean = %v, want 150", got.Display())
	}
}

func TestStatsStdDevSingleValueMissing(t *testing.T) {
	ds := table.NewDataset("", "t")
	ds.AddColumn(table.Column{Name: "x", Kind: table.Numeric})
	ds.AppendRow(table.Row{"x": table.Num(5)})
	s := computeStats(ds, ds.Rows, "x")
	if !s.stdVal().IsMissing() {
		t.Errorf("StdDev of single value should be missing, got %v", s.stdVal().Display())
	}
}

func TestMeansSumStat(t *testing.T) {
	// Selecting `sum` adds a Sum column with the group totals; selection order is
	// honored and only requested stats appear.
	sum, _ := meansStat("sum")
	n, _ := meansStat("n")
	res := buildMeansResult(scoresDS(), []string{"score"}, []string{"grp"}, []meanStat{sum, n}, -1, nil, nil)
	if names := res.ColumnNames(); len(names) != 4 || names[2] != "Sum" || names[3] != "N" {
		t.Fatalf("columns = %v, want [grp Variable Sum N] (selection order)", names)
	}
	byGrp := map[string]table.Row{}
	for _, r := range res.Rows {
		byGrp[res.Get(r, "grp").Str] = r
	}
	if got := res.Get(byGrp["a"], "Sum"); got.Num != 60 { // 10+20+30
		t.Errorf("group a Sum = %v, want 60", got.Display())
	}
	if got := res.Get(byGrp["b"], "Sum"); got.Num != 300 { // 100+200
		t.Errorf("group b Sum = %v, want 300", got.Display())
	}
}

func TestMeansMaxdecFormat(t *testing.T) {
	// maxdec=2 attaches a w.2 format to the float stats but leaves N integer.
	mean, _ := meansStat("mean")
	n, _ := meansStat("n")
	res := buildMeansResult(scoresDS(), []string{"score"}, nil, []meanStat{n, mean}, 2, nil, nil)
	var nFmt, meanFmt string
	for _, c := range res.Columns {
		switch c.Name {
		case "N":
			nFmt = c.Format
		case "Mean":
			meanFmt = c.Format
		}
	}
	if nFmt != "" {
		t.Errorf("N format = %q, want empty (counts stay integer)", nFmt)
	}
	if meanFmt != "14.2" {
		t.Errorf("Mean format = %q, want 14.2 (maxdec=2)", meanFmt)
	}
}

// TestMeansUserFormatClass verifies CLASS grouping by a user VALUE format:
// ages <30 -> "Young" (n=2), >=30 -> "Older" (n=3).
func TestMeansUserFormatClass(t *testing.T) {
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
	ds.AddColumn(table.Column{Name: "score", Kind: table.Numeric})
	add := func(a, s float64) { ds.AppendRow(table.Row{"age": table.Num(a), "score": table.Num(s)}) }
	add(22, 80)
	add(25, 90)
	add(40, 70)
	add(55, 60)
	add(33, 100)
	res := buildMeansResult(ds, []string{"score"}, []string{"age"}, defaultMeanStats(), -1, cat, map[string]string{})
	if res.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2 (Young, Older)", res.NObs())
	}
	if got := res.Get(res.Rows[0], "age").Str; got != "Young" {
		t.Errorf("group0 = %q, want Young", got)
	}
	if got := res.Get(res.Rows[0], "N").Num; got != 2 {
		t.Errorf("Young N = %v, want 2", got)
	}
	if got := res.Get(res.Rows[1], "age").Str; got != "Older" {
		t.Errorf("group1 = %q, want Older", got)
	}
	if got := res.Get(res.Rows[1], "N").Num; got != 3 {
		t.Errorf("Older N = %v, want 3", got)
	}
}

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
	res := buildMeansResult(scoresDS(), []string{"score"}, nil)
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
	res := buildMeansResult(scoresDS(), []string{"score"}, []string{"grp"})
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

package runtime

import (
	"testing"

	"github.com/solifugus/ass/table"
)

// byDataset builds a dataset with the given single character column values.
func byDataset(col string, vals ...string) *table.Dataset {
	ds := table.NewDataset("", "t")
	ds.AddColumn(table.Column{Name: col, Kind: table.Character})
	for _, v := range vals {
		ds.AppendRow(table.Row{col: table.Char(v)})
	}
	return ds
}

func TestComputeByGroupsSingleKey(t *testing.T) {
	// Groups: [A A] [B] [C C]
	ds := byDataset("g", "A", "A", "B", "C", "C")
	flags := ComputeByGroups(ds, []string{"g"})

	wantFirst := []bool{true, false, true, true, false}
	wantLast := []bool{false, true, true, false, true}
	for i := range flags {
		if flags[i].First[0] != wantFirst[i] {
			t.Errorf("row %d first.g = %v, want %v", i, flags[i].First[0], wantFirst[i])
		}
		if flags[i].Last[0] != wantLast[i] {
			t.Errorf("row %d last.g = %v, want %v", i, flags[i].Last[0], wantLast[i])
		}
	}
}

func TestComputeByGroupsTwoKeys(t *testing.T) {
	// Sorted by dept then team:
	//  row dept team
	//   0   A   x
	//   1   A   x
	//   2   A   y
	//   3   B   x
	ds := table.NewDataset("", "t")
	ds.AddColumn(table.Column{Name: "dept", Kind: table.Character})
	ds.AddColumn(table.Column{Name: "team", Kind: table.Character})
	add := func(d, tm string) { ds.AppendRow(table.Row{"dept": table.Char(d), "team": table.Char(tm)}) }
	add("A", "x")
	add("A", "x")
	add("A", "y")
	add("B", "x")

	flags := ComputeByGroups(ds, []string{"dept", "team"})

	// first.dept, first.team
	wantFirstDept := []bool{true, false, false, true}
	wantFirstTeam := []bool{true, false, true, true}
	// last.dept, last.team
	wantLastDept := []bool{false, false, true, true}
	wantLastTeam := []bool{false, true, true, true}

	for i := range flags {
		if flags[i].First[0] != wantFirstDept[i] {
			t.Errorf("row %d first.dept = %v, want %v", i, flags[i].First[0], wantFirstDept[i])
		}
		if flags[i].First[1] != wantFirstTeam[i] {
			t.Errorf("row %d first.team = %v, want %v", i, flags[i].First[1], wantFirstTeam[i])
		}
		if flags[i].Last[0] != wantLastDept[i] {
			t.Errorf("row %d last.dept = %v, want %v", i, flags[i].Last[0], wantLastDept[i])
		}
		if flags[i].Last[1] != wantLastTeam[i] {
			t.Errorf("row %d last.team = %v, want %v", i, flags[i].Last[1], wantLastTeam[i])
		}
	}
}

func TestComputeByGroupsSingleRow(t *testing.T) {
	ds := byDataset("g", "A")
	flags := ComputeByGroups(ds, []string{"g"})
	if !flags[0].First[0] || !flags[0].Last[0] {
		t.Errorf("single row should be both first and last: %+v", flags[0])
	}
}

//go:build cgo

package sql

import (
	"testing"

	"github.com/solifugus/ass/table"
)

func peopleLib() *table.Library {
	lib := table.NewLibrary()
	ds := table.NewDataset("", "people")
	ds.AddColumn(table.Column{Name: "name", Kind: table.Character})
	ds.AddColumn(table.Column{Name: "age", Kind: table.Numeric})
	ds.AppendRow(table.Row{"name": table.Char("John"), "age": table.Num(25)})
	ds.AppendRow(table.Row{"name": table.Char("Mary"), "age": table.Num(30)})
	ds.AppendRow(table.Row{"name": table.Char("Tim"), "age": table.Num(12)})
	lib.Put(ds)
	return lib
}

func TestEngineSelectWhere(t *testing.T) {
	eng, err := NewEngine(peopleLib())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	ds, err := eng.Query("select name, age from people where age >= 18 order by age")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if ds.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2", ds.NObs())
	}
	if got := ds.Get(ds.Rows[0], "name"); got.Str != "John" {
		t.Errorf("row0 name = %q, want John", got.Str)
	}
	// age column came back numeric.
	for _, c := range ds.Columns {
		if c.Name == "age" && c.Kind != table.Numeric {
			t.Errorf("age column kind = %v, want numeric", c.Kind)
		}
	}
}

func TestEngineAggregate(t *testing.T) {
	eng, _ := NewEngine(peopleLib())
	defer eng.Close()

	ds, err := eng.Query("select count(*) as n, avg(age) as a from people")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if ds.NObs() != 1 {
		t.Fatalf("NObs = %d, want 1", ds.NObs())
	}
	if got := ds.Get(ds.Rows[0], "n"); got.Num != 3 {
		t.Errorf("count = %v, want 3", got.Display())
	}
	if got := ds.Get(ds.Rows[0], "a"); got.Num != (25.0+30+12)/3 {
		t.Errorf("avg = %v, want %v", got.Display(), (25.0+30+12)/3)
	}
}

func TestEngineCreateTableAndSave(t *testing.T) {
	lib := peopleLib()
	eng, _ := NewEngine(lib)
	defer eng.Close()

	if err := eng.Exec("create table adults as select * from people where age >= 18"); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if err := eng.Save(lib, "adults"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	adults, ok := lib.Get("adults")
	if !ok {
		t.Fatal("adults not saved to library")
	}
	if adults.NObs() != 2 {
		t.Errorf("adults NObs = %d, want 2", adults.NObs())
	}
}

func TestEngineNullIsMissing(t *testing.T) {
	lib := table.NewLibrary()
	ds := table.NewDataset("", "t")
	ds.AddColumn(table.Column{Name: "x", Kind: table.Numeric})
	ds.AppendRow(table.Row{"x": table.Num(1)})
	ds.AppendRow(table.Row{"x": table.MissingNum()})
	lib.Put(ds)

	eng, _ := NewEngine(lib)
	defer eng.Close()
	res, err := eng.Query("select x from t order by x")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	// NULL sorts first in SQLite; it should read back as numeric missing.
	if got := res.Get(res.Rows[0], "x"); !got.IsMissing() {
		t.Errorf("row0 x = %v, want missing", got.Display())
	}
}

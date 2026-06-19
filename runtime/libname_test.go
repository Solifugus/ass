package runtime

import (
	"strings"
	"testing"

	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/table"
)

// fakeBackend is an in-memory table.Backend standing in for a database so the
// LIBNAME routing can be tested without a live connection.
type fakeBackend struct {
	tables map[string]*table.Dataset
	loads  int
}

func (f *fakeBackend) Load(member string) (*table.Dataset, bool, error) {
	f.loads++
	ds, ok := f.tables[strings.ToLower(member)]
	return ds, ok, nil
}

func customersDS() *table.Dataset {
	ds := table.NewDataset("PG", "customers")
	ds.AddColumn(table.Column{Name: "id", Kind: table.Numeric})
	ds.AddColumn(table.Column{Name: "name", Kind: table.Character})
	ds.AppendRow(table.Row{"id": table.Num(1), "name": table.Char("Acme")})
	ds.AppendRow(table.Row{"id": table.Num(2), "name": table.Char("Globex")})
	return ds
}

func TestResolveExternalBackend(t *testing.T) {
	lib := table.NewLibrary()
	fb := &fakeBackend{tables: map[string]*table.Dataset{"customers": customersDS()}}
	lib.Assign("pg", fb)

	if !lib.IsExternal("pg.customers") {
		t.Error("pg.customers should be external")
	}
	if lib.IsExternal("customers") {
		t.Error("unqualified name should not be external")
	}
	ds, ok, err := lib.Resolve("PG.Customers")
	if err != nil || !ok || ds.NObs() != 2 {
		t.Fatalf("Resolve(pg.customers) = %v ok=%v err=%v", ds, ok, err)
	}
	lib.Unassign("pg")
	if lib.IsExternal("pg.customers") {
		t.Error("after clear, pg should no longer be external")
	}
}

func TestSetFromExternalLibref(t *testing.T) {
	lib := table.NewLibrary()
	lib.Assign("pg", &fakeBackend{tables: map[string]*table.Dataset{"customers": customersDS()}})

	prog := parser.New("data out; set pg.customers; keep id name; run;").ParseProgram()
	var b strings.Builder
	if err := RunProgram(prog, lib, log.New(&b)); err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
	out, ok := lib.Get("out")
	if !ok {
		t.Fatal("OUT not created")
	}
	if out.NObs() != 2 {
		t.Errorf("OUT nobs = %d, want 2", out.NObs())
	}
	if got := out.Get(out.Rows[1], "name"); got.Str != "Globex" {
		t.Errorf("row 2 name = %q, want Globex", got.Str)
	}
}

func TestProcReadFromExternalLibref(t *testing.T) {
	lib := table.NewLibrary()
	lib.Assign("pg", &fakeBackend{tables: map[string]*table.Dataset{"customers": customersDS()}})

	// PROC on a DB-qualified data= must route through the backend without the
	// proc knowing about libraries, and must not leak a temp into WORK. (PROC
	// MEANS prints to stdout; its format is covered by proc tests — here we only
	// assert the routing succeeds and leaves WORK clean.)
	prog := parser.New("proc means data=pg.customers; var id; run;").ParseProgram()
	if err := RunProgram(prog, lib, log.New(&strings.Builder{})); err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
	for _, n := range lib.Names() {
		if strings.HasPrefix(n, "_DATAOPT_") {
			t.Errorf("temp dataset leaked into WORK: %s", n)
		}
	}
}

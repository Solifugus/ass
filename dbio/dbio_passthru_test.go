//go:build cgo

package dbio

import (
	"testing"

	"github.com/solifugus/ass/table"
)

// TestPassthroughQueryExecDrop exercises the PROC SQL pass-through engine path —
// QuerySQL (native SELECT back to a dataset), ExecSQL (native DDL/DML), and Drop
// — against a real SQLite backend, so the behavior is verified without a server.
func TestPassthroughQueryExecDrop(t *testing.T) {
	be := newSQLiteBackend(t)
	if err := be.Store(sampleDataset()); err != nil {
		t.Fatalf("seed Store: %v", err)
	}

	// QuerySQL: a native aggregate the in-process engine never sees.
	ds, err := be.QuerySQL(`SELECT count(*) AS n, sum(balance) AS total FROM "customers"`)
	if err != nil {
		t.Fatalf("QuerySQL: %v", err)
	}
	if ds.NObs() != 1 {
		t.Fatalf("nobs = %d, want 1", ds.NObs())
	}
	if v := ds.Get(ds.Rows[0], "n"); v.Num != 3 {
		t.Errorf("count = %v, want 3", v.Display())
	}
	if v := ds.Get(ds.Rows[0], "total"); v.IsMissing() || v.Num != 1234.0 { // 1234.50 + (-0.5)
		t.Errorf("sum(balance) = %v, want 1234", v.Display())
	}

	// ExecSQL: native DML the engine runs verbatim.
	if err := be.ExecSQL(`DELETE FROM "customers" WHERE id = 1`); err != nil {
		t.Fatalf("ExecSQL delete: %v", err)
	}
	after, ok, err := be.Load("customers")
	if err != nil || !ok {
		t.Fatalf("Load after delete: ok=%v err=%v", ok, err)
	}
	if after.NObs() != 2 {
		t.Errorf("nobs after delete = %d, want 2", after.NObs())
	}

	// Drop: removes the table entirely.
	if err := be.Drop("customers"); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if _, ok, _ := be.Load("customers"); ok {
		t.Errorf("table still present after Drop")
	}
}

// TestPassthroughBackendInterfaces verifies the SQLite backend satisfies the
// pass-through and drop interfaces PROC SQL type-asserts to.
func TestPassthroughBackendInterfaces(t *testing.T) {
	be := newSQLiteBackend(t)
	if _, ok := interface{}(be).(table.SQLBackend); !ok {
		t.Error("Backend does not implement table.SQLBackend")
	}
	if _, ok := interface{}(be).(table.DropBackend); !ok {
		t.Error("Backend does not implement table.DropBackend")
	}
}

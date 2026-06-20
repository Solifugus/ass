package dbio

import (
	"errors"
	"testing"

	"github.com/solifugus/ass/table"
)

// These tests cover the DB2 dialect choices without needing the CGo driver or a
// live server (they exercise the pure mapping helpers), so they run in the
// default build alongside every other engine's dialect.

func TestDB2ColumnType(t *testing.T) {
	b := &Backend{engine: "db2"}
	cases := []struct {
		col  table.Column
		want string
	}{
		{table.Column{Kind: table.Character, Length: 20}, "VARCHAR(20)"},
		{table.Column{Kind: table.Character}, "VARCHAR(255)"}, // unsized falls back
		{table.Column{Kind: table.Numeric}, "DOUBLE"},
		{table.Column{Kind: table.Numeric, Format: "date9."}, "DATE"},
		{table.Column{Kind: table.Numeric, Format: "datetime."}, "TIMESTAMP"},
	}
	for _, c := range cases {
		if got := b.columnType(c.col); got != c.want {
			t.Errorf("columnType(%+v) = %q, want %q", c.col, got, c.want)
		}
	}
}

func TestDB2Placeholders(t *testing.T) {
	b := &Backend{engine: "db2"}
	// DB2 (IBM CLI driver) uses positional `?` markers.
	if got := b.placeholders(3); got != "?, ?, ?" {
		t.Errorf("placeholders(3) = %q, want %q", got, "?, ?, ?")
	}
}

func TestDB2QuoteIdent(t *testing.T) {
	if got := quoteIdent("db2", "fruit"); got != `"fruit"` {
		t.Errorf("quoteIdent = %q, want %q", got, `"fruit"`)
	}
	if got := quoteIdent("db2", "myschema.fruit"); got != `"myschema"."fruit"` {
		t.Errorf("quoteIdent (schema) = %q, want %q", got, `"myschema"."fruit"`)
	}
}

func TestDB2MissingTableErr(t *testing.T) {
	// DB2 reports an absent table as SQL0204N / SQLSTATE 42704.
	for _, msg := range []string{
		`SQLCODE=-204, SQLSTATE=42704, "DB2INST1.FRUIT" is an undefined name`,
		`SQL0204N  "DB2INST1.FRUIT" is an undefined name.`,
	} {
		if !isMissingTableErr(errors.New(msg)) {
			t.Errorf("isMissingTableErr(%q) = false, want true", msg)
		}
	}
	if isMissingTableErr(errors.New("SQL0911N deadlock or timeout")) {
		t.Error("a non-missing-table error was misclassified as missing")
	}
}

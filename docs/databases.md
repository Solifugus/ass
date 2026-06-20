# External databases (LIBNAME engines)

ASS can read tables from relational databases the SAS way — through a `LIBNAME`
engine. You assign a libref to a database, then reference its tables as datasets
anywhere a dataset is expected (`set`, `merge`, `data=`, …). This mirrors the
SAS/ACCESS LIBNAME-engine model.

> **Status: read and write.** Reading database tables as datasets is supported,
> and a DATA step can write a dataset back to a database library
> (`data pg.newtab; set …;`), replacing the table. Streaming for very large
> tables and PROC SQL pass-through remain future work.

## Assigning a library

```sas
libname pg postgres "postgres://user:pass@localhost:5432/sales?sslmode=disable";

data recent;
  set pg.orders(where=(order_date >= '01JAN2024'd) keep=id customer amount order_date);
run;

proc print data=pg.customers;
run;

libname pg clear;   /* unassign */
```

The general form is:

```
libname <ref> <engine> "<connection-string>";
libname <ref> clear;
```

The connection string is passed to the underlying driver verbatim (a DSN or
URL — whatever that driver accepts).

## Supported engines

| Engine name(s) | Driver | Pure Go? | Notes |
|----------------|--------|----------|-------|
| `postgres`, `postgresql` | `jackc/pgx` | ✅ | DSN or URL form (`postgres://…` or `host=… dbname=…`). |
| `sqlserver`, `mssql` | `microsoft/go-mssqldb` | ✅ | URL form (`sqlserver://user:pass@host?database=db`). |
| `oracle` | `sijms/go-ora` | ✅ | URL form (`oracle://user:pass@host:1521/service`). |
| `sqlite`, `sqlite3` | `mattn/go-sqlite3` | ❌ CGo | A single database file (or `:memory:`): `libname db sqlite "/path/file.db";`. Registered only in CGo builds (the project already requires CGo for PROC SQL); ideal for local files and as the write-back path that needs no server. |
| `db2` | *(not built in)* | ❌ CGo | Needs the IBM CLI driver (`ibmdb/go_ibm_db`); excluded so the default build stays `CGO_ENABLED=0`. A future build tag will add it. |

Because all engines go through Go's `database/sql`, adding another is mostly a
driver import plus its dialect quirks.

## Writing back (DATA step → database)

A DATA step whose output is qualified with a database libref writes to that
database instead of the in-memory WORK store:

```sas
libname pg postgres "postgres://user:pass@localhost:5432/sales?sslmode=disable";

data pg.high_value;          /* creates/replaces table HIGH_VALUE in Postgres */
  set work.orders;
  where amount >= 1000;
run;
```

Semantics:

- **Replace.** The target table is dropped and recreated to match the dataset's
  columns, then all rows are inserted — SAS dataset-replacement semantics. The
  whole operation runs in **one transaction**, so a failure leaves the existing
  table untouched.
- **Type mapping (SAS → database).** Character columns become the engine's
  variable text type sized by the column length (`VARCHAR(n)` / `NVARCHAR(n)` /
  `VARCHAR2(n)`); numeric columns become the engine's double type
  (`DOUBLE PRECISION` / `FLOAT` / `BINARY_DOUBLE` / `REAL`); a numeric column
  whose format is a date or datetime format is written to a real `DATE` /
  `TIMESTAMP` (`DATETIME2` on SQL Server), converting the SAS day/second number
  back to a calendar value. Missing values become SQL `NULL`.
- **Read-only libraries** (a base/directory `.sas7bdat` libref) reject a write
  with a clear error.
- Not yet: appending to an existing table (`mod`-style), `proc sort out=db.x` /
  `proc sql create table db.x`, and per-column type overrides.

## Type mapping (database → SAS)

SAS has two storage types (numeric double, fixed character) plus formats. ASS
maps columns on read:

| Database type | SAS | Notes |
|---------------|-----|-------|
| INTEGER, BIGINT, NUMERIC, DECIMAL, REAL, DOUBLE | numeric | DECIMAL/NUMERIC returned as text by some drivers are parsed. |
| BOOLEAN | numeric | `true`→1, `false`→0. |
| CHAR, VARCHAR, TEXT, CLOB, UUID, JSON, XML | character | |
| DATE | numeric | SAS date (days since 1960-01-01), formatted `date9.`. |
| TIMESTAMP / DATETIME | numeric | SAS datetime (seconds since 1960-01-01), formatted `datetime.`. |
| NULL (any) | missing | typed numeric/character missing. |

## Compatibility notes

- **Same results, computed locally.** ASS reads the table and processes it in the
  DATA step / PROC. SAS can *push down* `WHERE`/joins/aggregation to the database
  for speed (implicit pass-through); ASS does not yet — results are identical, but
  large tables transfer in full. Use a dataset-option `where=`/`keep=` to limit
  what is materialized.
- **Explicit PROC SQL pass-through** (`connect to … ; select … from connection
  to …`) is not yet wired to external librefs (the in-process SQLite engine still
  backs `proc sql`); it is a natural follow-on given the shared `database/sql`
  layer.
- The long tail of SAS/ACCESS LIBNAME options (`SCHEMA=`, `READBUFF=`,
  `PRESERVE_TAB_NAMES=`, bulk loaders, …) is not implemented. Schema-qualified
  member names (`pg.'schema.table'n`) are partially handled via a single dot.

## Running the integration tests

The **SQLite** engine makes the full read *and write* path testable with no
server: `go test ./dbio/ -run TestSQLite` round-trips a dataset (numeric,
character, date, and missing values) through `Store` then `Load`, and
`go test ./runtime/ -run TestDataStepWriteBack` drives `data db.x; set …;`
end-to-end against a temp database file. The corpus item
`data_step_db_writeback_001` value-verifies the same path through `ass test`.

A real-database test for **Postgres** type mapping is gated on an env var:

```bash
ASS_PG_DSN="postgres://user:pass@localhost:5432/dbname?sslmode=disable" \
    go test ./dbio/ -run TestPostgresIntegration -v
```

It creates a throwaway table, reads it through the backend, asserts the SAS value
mapping, and drops the table.

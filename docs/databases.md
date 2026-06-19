# External databases (LIBNAME engines)

ASS can read tables from relational databases the SAS way — through a `LIBNAME`
engine. You assign a libref to a database, then reference its tables as datasets
anywhere a dataset is expected (`set`, `merge`, `data=`, …). This mirrors the
SAS/ACCESS LIBNAME-engine model.

> **Status: read-only.** Reading database tables as datasets is supported.
> Writing datasets back to a database (`data pg.newtab; …`) is planned next.

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
| `db2` | *(not built in)* | ❌ CGo | Needs the IBM CLI driver (`ibmdb/go_ibm_db`); excluded so the default build stays `CGO_ENABLED=0`. A future build tag will add it. |

Because all engines go through Go's `database/sql`, adding another is mostly a
driver import plus its dialect quirks.

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

## Running the integration test

Unit tests cover the LIBNAME routing with an in-memory fake backend (no database
needed). A real-database test for Postgres type mapping is gated on an env var:

```bash
ASS_PG_DSN="postgres://user:pass@localhost:5432/dbname?sslmode=disable" \
    go test ./dbio/ -run TestPostgresIntegration -v
```

It creates a throwaway table, reads it through the backend, asserts the SAS value
mapping, and drops the table.

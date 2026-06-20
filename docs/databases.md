# External databases (LIBNAME engines)

ASS can read tables from relational databases the SAS way — through a `LIBNAME`
engine. You assign a libref to a database, then reference its tables as datasets
anywhere a dataset is expected (`set`, `merge`, `data=`, …). This mirrors the
SAS/ACCESS LIBNAME-engine model.

> **Status: read, write, and PROC SQL pass-through.** Reading database tables as
> datasets is supported, a DATA step can write a dataset back to a database
> library (`data pg.newtab; set …;`), replacing the table, and PROC SQL can send
> a database its own native SQL (`connect to` / `from connection to` / `execute`)
> — see [PROC SQL pass-through](#proc-sql-pass-through) below. A value-safe subset
> of `keep=`/`where=` is pushed down to the database (column projection + numeric
> filters); streaming for very large tables and pushing joins/aggregation remain
> future work.

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
| `db2` | `ibmdb/go_ibm_db` | ❌ CGo + CLI driver, `-tags db2` | IBM Db2 (LUW). Connection string is the CLI form: `HOSTNAME=host;PORT=50000;DATABASE=db;UID=user;PWD=pw`. Registered only under the `db2` build tag (see below) because the driver links IBM's native CLI driver, which the default build must not require. |

### Building the DB2 engine

The DB2 driver is CGo and links against IBM's **CLI driver** (`clidriver`), a
native library the driver's installer downloads. To keep a plain
`go build ./...` free of that dependency, DB2 is gated behind the `db2` build
tag — `dbio/dbio_db2.go` (which imports `go_ibm_db` and registers the engine) is
compiled only with `-tags db2`. One-time setup:

```bash
# 1. Add the module (already in go.mod) and download the CLI driver:
go run github.com/ibmdb/go_ibm_db/installer/setup.go   # extracts clidriver into the module cache

# 2. Point the toolchain and loader at it (the installer prints these paths):
export IBM_DB_HOME="$(go env GOMODCACHE)/github.com/ibmdb/clidriver"
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib -ldb2"
export LD_LIBRARY_PATH="$IBM_DB_HOME/lib:$LD_LIBRARY_PATH"

# 3. Build / run with the tag:
go build -tags db2 ./...
go run -tags db2 ./cmd/ass run program.sas
```

`libdb2.so` also needs `libxml2.so.2` (the older libxml2 ABI). Distributions
that ship only `libxml2.so.16` (e.g. recent Ubuntu) won't have it; install an
older libxml2 or place a `libxml2.so.2` on `LD_LIBRARY_PATH`. A default
`go mod tidy` ignores the `db2` tag and will try to drop `go_ibm_db` from
`go.mod` — a comment there flags it; restore the line (or use `go mod tidy -e`)
if that happens.

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
- **Append.** `PROC APPEND base=db.x data=…` adds rows to an existing table
  in place — an INSERT-only write, not a drop-and-recreate (see below).
- Not yet: per-column type overrides.

### PROC output to a database

PROCs that produce a dataset can also target a database libref, with the same
replace-in-one-transaction semantics as the DATA step:

```sas
libname db sqlite "/path/sales.db";

proc sort data=work.orders out=db.sorted;   /* writes table SORTED to the DB */
  by id;
run;

proc sql;                                    /* materializes TOTALS in the DB */
  create table db.totals as
    select region, sum(amount) as total
    from work.orders
    group by region;
quit;
```

PROC SORT's `out=` and PROC SQL's `create table <libref>.<name> as …` both route
through `table.Library.Store`, the single write-routing point (`StoreExternal` →
`Backend.Store` for a database libref, the WORK store otherwise) shared with the
DATA step. PROC SQL builds the result under a temporary in-engine name first
(since a `libref.name` target can't be created directly in the embedded SQLite),
then stores the materialized dataset to the bound engine.

**PROC APPEND** adds the observations of `data=` to the end of `base=`, where
either may be a database libref:

```sas
proc append base=db.fact data=work.daily;   /* INSERT daily rows into FACT */
run;
```

If `base=` does not exist it is created from `data=` (the first load); otherwise
its rows are appended. For a database `base=`, the append is an **in-place
INSERT** — not a drop-and-recreate — so existing rows and the table definition are
untouched; this is the `mod`-style incremental-load path. It routes through
`table.Library.Append` → `Backend.Append` (a `WriteBackend` that also implements
the optional `AppendBackend` interface; a plain `WriteBackend` falls back to
load-combine-replace). `FORCE` permits appending when `data=` has variables
`base=` lacks (dropped) or a type disagrees (set missing), matching SAS; without
`FORCE` such a mismatch refuses the append.

## PROC SQL pass-through

Pass-through sends a database its **own native SQL**, run by the database engine
rather than ASS's in-process SQLite. Use it for DBMS-specific SQL, server-side
work on large tables, or DDL/DML that should execute remotely.

```sas
libname ora oracle "oracle://system:pw@localhost:1521/FREEPDB1";

proc sql;
  /* Reuse the assigned libref's connection — no `connect to` needed. */
  create table work.by_region as
    select * from connection to ora
      (select region, count(*) AS n, sum(amount) AS total
         from sales group by region);

  /* Run native DDL/DML on the database (no result set). */
  execute (delete from staging where loaded = 1) by ora;
quit;
```

Or open a connection explicitly within the block:

```sas
proc sql;
  connect to oracle as o (connection="oracle://system:pw@localhost:1521/FREEPDB1");
  select * from connection to o (select * from dual);   /* prints the listing  */
  disconnect from o;
quit;
```

Semantics:

- **`connect to <engine> [as <alias>] (connection="<conn-string>")`** opens a
  connection for the block, named by `<alias>` (or the engine name if no `as`).
  The connection string is the same LIBNAME-style string the engine accepts
  (`dsn=` and `connect=` are accepted as synonyms for `connection=`). A
  connection opened this way is closed at `disconnect from <alias>` or when the
  PROC SQL block ends.
- **Reusing a libref.** If a libref is already assigned with `libname`, you can
  skip `connect to` entirely and name that libref directly in `from connection
  to <libref>` / `execute (...) by <libref>` — its existing connection is reused
  (and left open; the LIBNAME owns it).
- **`select … from connection to <name> (<native query>)`** runs the native
  query on the database and brings its result set back as a dataset. A bare
  select prints the listing; wrapping it in `create table <tgt> as select * from
  connection to …` materializes the result into `<tgt>` (any library, via the
  shared `Library.Store` routing). The native SQL is sent verbatim — ASS does not
  parse or rewrite it; the returned columns map to SAS types exactly as a read
  does (text→character, DATE/TIMESTAMP→SAS date/datetime numeric, NULL→missing).
- **`execute (<native sql>) by <name>`** runs a no-result statement (DDL/DML) on
  the database verbatim.
- **`drop table <libref>.<member>`** inside PROC SQL is routed to the external
  database (it drops the real table), instead of hitting the in-process SQLite
  engine. (A `drop` of a WORK/embedded table is unaffected.)

The pass-through path goes through `table.SQLBackend`
(`QuerySQL`/`ExecSQL`) and `table.DropBackend` (`Drop`), implemented by every
database engine. Not yet: the outer `select` of a `from connection to` query is
expected to be `select *` (ASS returns the native result set as-is rather than
re-projecting it); `connect to` connection-option forms other than a connection
string (e.g. SAS's `user=`/`password=`/`path=` triplets).

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

- **Implicit query pushdown (value-safe subset).** A dataset-option `keep=` is
  pushed as a column projection and a `where=` of numeric comparisons using
  `=`/`>`/`>=` is pushed as a SQL `WHERE`, so the database returns only the needed
  columns/rows. The subset is chosen so the pushed predicate selects exactly the
  same rows SAS would: SAS orders a missing value below every number, so
  `=`/`>`/`>=` exclude missing just as SQL excludes NULL. Operators that *keep*
  missing in SAS (`<`, `<=`, `ne`), string/function predicates, `drop=`, and
  joins/aggregation are **not** pushed — they are computed locally after a full
  read. Either way the result is identical; pushdown only reduces transfer. ASS
  validates each pushed column is numeric (via a zero-row metadata probe) and
  falls back to a full read on anything it cannot prove safe.
- **Ordinary `proc sql` can read an external libref as a source.** The in-process
  SQLite engine backs ordinary `proc sql` (joins, group by, create-table-as-select),
  and a libref-qualified source — `select … from db.orders`, including a join of an
  external table with a WORK table — is loaded on demand from the bound engine into
  the in-process database and queried there. WORK-qualified sources (`work.x`)
  resolve too. (Source resolution rewrites only qualifiers that name a real library
  — WORK or an assigned libref — so `alias.column` references like `t.amt` are left
  untouched.) This is value-only loading, not pushdown: the external table is read
  in full, then the join/aggregation runs locally. For server-side execution use
  pass-through below.
- **Explicit PROC SQL pass-through** (`connect to … ; select … from connection
  to …`) **is supported** — see [PROC SQL pass-through](#proc-sql-pass-through).
  Pass-through is the escape hatch that sends native SQL to the database itself
  (DBMS-specific dialect, server-side joins/aggregation).
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
PROC SQL pass-through is likewise SQLite-testable with no server:
`go test ./dbio/ -run TestPassthrough` covers `QuerySQL`/`ExecSQL`/`Drop`, and
`go test ./proc/ -run TestPassthrough` drives `connect to` / `from connection
to` / `execute` / `drop table <libref>.x` end-to-end; the corpus item
`proc_sql_passthrough_001` value-verifies a `from connection to` result.

A real-database test for **Postgres** type mapping is gated on an env var:

```bash
ASS_PG_DSN="postgres://user:pass@localhost:5432/dbname?sslmode=disable" \
    go test ./dbio/ -run TestPostgresIntegration -v
```

It creates a throwaway table, reads it through the backend, asserts the SAS value
mapping, and drops the table.

The **SQL Server** and **Oracle** write paths have matching env-gated tests
(`TestSQLServerIntegration` / `TestOracleIntegration`). Each writes a dataset
through `Store`, reads it back through `Load` (asserting the type mapping, the
`DATE` round-trip, and NULL→missing), and the Oracle test additionally appends a
row through `Append` to confirm the in-place INSERT on a live server:

```bash
ASS_MSSQL_DSN="sqlserver://user:pass@host:1433?database=db&encrypt=disable" \
    go test ./dbio/ -run TestSQLServerIntegration -v

ASS_ORACLE_DSN="oracle://system:pass@localhost:1521/FREEPDB1" \
    go test ./dbio/ -run TestOracleIntegration -v
```

A throwaway Oracle is one container away (no SAS or Oracle account needed for the
community image):

```bash
podman run -d --name oracle-free -p 1521:1521 -e ORACLE_PASSWORD=pass \
    docker.io/gvenzl/oracle-free:slim     # ready when the log says "DATABASE IS READY TO USE!"
```

See [`oracle-test-sandbox.md`](oracle-test-sandbox.md) for a full cheat sheet on
running and inspecting this sandbox.

> **Oracle version note.** ASS's `Store` issues `DROP TABLE IF EXISTS` before
> recreating a table; `IF EXISTS` on DDL is an **Oracle 23ai** feature (what the
> `gvenzl/oracle-free` image runs). On older Oracle the replace path would need an
> ignore-ORA-00942 drop instead — not yet implemented.

The **DB2** write path has the same env-gated test (`TestDB2Integration`,
build-tagged `db2`): it writes through `Store`, reads back through `Load`
(type mapping, `DATE` round-trip, NULL→missing), then `Append`s a row in place.
Run it (with the CLI driver env from "Building the DB2 engine" set):

```bash
ASS_DB2_DSN="HOSTNAME=localhost;PORT=50000;DATABASE=testdb;UID=db2inst1;PWD=ass_test" \
    go test -tags db2 ./dbio/ -run TestDB2Integration -v
```

A throwaway DB2 is one container away (IBM's community image), but note it needs
**rootful** podman (or Docker): Db2's `db2start` runs setuid-root binaries, which
rootless podman blocks via its `nosuid` mounts (`SQL1641N`). The image also takes
several minutes to initialize:

```bash
sudo podman run -d --name db2 --privileged=true -p 50000:50000 \
    -e LICENSE=accept -e DB2INST1_PASSWORD=ass_test -e DBNAME=testdb \
    icr.io/db2_community/db2
# ready when this connects (not SQL1032N/SQL1035N):
sudo podman exec db2 su - db2inst1 -c "db2 connect to testdb"
```

The DB2 backend connects over TCP (`localhost:50000`), so once the server is up
the implement/test loop needs no further `sudo` — only container lifecycle does.

See [`db2-test-sandbox.md`](db2-test-sandbox.md) for the full DB2 sandbox cheat
sheet (lifecycle, the `libxml2.so.2` workaround, poking at it with the db2 CLP,
and every gotcha) and [`podman-cheatsheet.md`](podman-cheatsheet.md) for general
container ops.

> **DB2 version note.** DB2 has no `DROP TABLE IF EXISTS`, so `Store`'s replace
> step issues a plain `DROP TABLE` and treats a "table does not exist" error
> (`SQL0204N` / SQLSTATE 42704) as success — see `dropIfExists`.

# PROC SQL backend decision

**Date:** 2026-06-16
**Decision:** Embed **SQLite** via the CGo driver `github.com/mattn/go-sqlite3`.
**Decided by:** project owner (chosen over a Go-native mini-engine and over DuckDB).

## Options considered

1. **Go-native mini-engine** — hand-write a SQL parser + executor over the
   in-memory `table.Library`. No external deps, full clean-room control, no CGo.
   Rejected: large amount of code per SQL feature; coverage limited to what we
   implement.
2. **Embed SQLite (CGo)** — load library datasets into an in-memory SQLite DB,
   run real SQL, read results back. **Chosen.** Huge SQL coverage for little
   code; mature and ubiquitous. Cost: a CGo dependency (needs a C toolchain to
   build) and the SQL layer is not clean-room.
3. **Embed DuckDB** — columnar/analytics-oriented, closest to SAS PROC SQL use.
   Rejected for now: heaviest dependency and binary size; revisit if analytical
   SQL performance becomes a goal.

## Consequences

- **Build now requires CGo** (`CGO_ENABLED=1` and a C compiler, e.g. gcc).
  Cross-compilation and fully-static builds need the usual CGo care.
- The legal note still holds: SQLite is public and permissively licensed; the
  clean-room constraint applies to SAS, not to using SQLite as the SQL engine.
- Datasets are bridged by name (case-insensitive, identifiers left unquoted so
  SQLite's case-insensitive matching applies). Numeric↔REAL, character↔TEXT;
  numeric missing ↔ SQL NULL.

## Implementation

- `sql/engine.go` — `Engine` wraps an in-memory `:memory:` SQLite DB.
  `NewEngine(lib)` loads every dataset; `Query` returns results as a
  `table.Dataset` (column kinds inferred from returned values); `Exec` runs
  non-row statements; `Save` reads a created table back into the library.
- `proc/sql.go` — the PROC SQL proc: splits the verbatim `ProcStep.RawBody`
  into statements, prints bare `SELECT`s via the PROC PRINT listing renderer,
  and materializes `CREATE TABLE ... AS SELECT` into the library.
- The raw SQL body is captured verbatim from source (lexer `Token.Pos/End` +
  `Lexer.Slice`, surfaced as `ProcStep.RawBody`) because reconstructing it from
  SAS tokens loses string-literal quotes.

## Update (2026-06-23): swapped to the pure-Go driver `modernc.org/sqlite`

The SQLite **engine** choice (option 2) stands; only the **driver** changed. We
replaced the CGo `mattn/go-sqlite3` with the pure-Go `modernc.org/sqlite`
(transpiled SQLite, used through `database/sql`). This removes the C-toolchain
requirement entirely: the whole engine — PROC SQL and the `sqlite` LIBNAME
engine — now builds with `CGO_ENABLED=0` into a fully static binary, which was
the main cost of the original decision (consequence "Build now requires CGo").

Rationale and de-risking:
- The driver swap was independent of every SAS-facing semantic above (the bridge,
  name handling, type mapping are unchanged).
- The one risk — big-endian correctness on s390x/LinuxONE — was verified resolved
  (2026-06-23): `modernc.org/sqlite v1.53.0` passes a focused big-endian test on
  `linux/s390x` (see `docs/design.md` §15).
- The driver is registered as `"sqlite"` (modernc) rather than `"sqlite3"` (mattn);
  `sql/engine.go` and `dbio/dbio_sqlite.go` open it by that name.

Consequence: the `//go:build cgo` gating, the pure-Go PROC SQL stub
(`proc/sql_nocgo.go`), and the corpus skip logic for SQL items were all removed —
PROC SQL is now always available. DB2 remains the only CGo engine (`-tags db2`).

## Known limitations (future work)

- `splitStatements` splits on `;` literally — a semicolon inside a SQL string
  literal would split wrongly. No corpus item hits this yet.
- SAS-specific PROC SQL extensions (e.g. `calculated`, `monotonic()`, dictionary
  tables, `INTO :macrovar`) are not translated.
- SAS function names that differ from SQLite's are not yet shimmed.

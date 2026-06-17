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

## Known limitations (future work)

- `splitStatements` splits on `;` literally — a semicolon inside a SQL string
  literal would split wrongly. No corpus item hits this yet.
- SAS-specific PROC SQL extensions (e.g. `calculated`, `monotonic()`, dictionary
  tables, `INTO :macrovar`) are not translated.
- SAS function names that differ from SQLite's are not yet shimmed.

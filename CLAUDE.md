# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project status

Implementation is well advanced and tracked in **`docs/PLAN.md`** (a living, resumable plan — read its "How to resume" section first; the Progress log at the bottom is the source of truth for what's done). As of the latest work, **Phases 0–13 are complete, including the 12.3 PROC REG/GLM stretch**, and the post-roadmap deferral backlog has been worked through (see the 2026-06-17 progress-log entries). The full lexer → macro → parser → runtime pipeline runs real programs end-to-end (`ass file.sas` or `ass run file.sas`): the DATA step (input/datalines with informats, infile for external flat files (dlm=/dsd/firstobs=/obs=), file/put for writing flat files (dlm=/dsd, list/literal/formatted output, data _null_), set, merge/in=, dataset options keep=/drop=/rename=/where=, if/then/else, subsetting if, where, do-loops, retain, sum, arrays, BY-group first./last., keep/drop/format, output), ~35 functions with missing-value + coercion semantics, PROC PRINT/SORT/SQL (SQLite)/MEANS/SUMMARY/FREQ (one- & two-way)/REG/GLM (incl. CLASS via reference-cell coding)/FORMAT (user VALUE formats), the macro preprocessor, formats, and date literals. External-database **LIBNAME engines** (read-only) let a `libname` bind a libref to Postgres/SQL Server/Oracle so DBMS tables read as datasets (`set pg.orders;`, `proc print data=pg.customers;`) — see `docs/databases.md`. The `ass test` harness reports **34/34 corpus items at 100%** and value-verifies a subset against hand-derived dataset values.

Companion docs: `README.md` (usage/features), `docs/COMPATIBILITY.md` (matrix + deferrals), `docs/CONTRIBUTING.md` (how to extend), `docs/design.md` (rationale), `docs/databases.md` (external-database LIBNAME engines), `docs/PLAN.md` (development log). The architecture notes below remain the conceptual map. Known deferrals (tracked in `docs/PLAN.md`), all now forward-looking rather than gaps: column/pointer input; the SAS-fidelity GLM upgrade (sweep/generalized-inverse parameterization, Type I/III SS, LSMEANS — gated on a real-SAS reference; CLASS effects already work via reference-cell coding); JSON harness output; incremental `expected.datasets` backfill. (`proc format` user VALUE formats, two-way PROC FREQ, PROC REG `Pr>|t|`, list-input informats, dataset options, GLM CLASS effects, and the value-verification harness are all implemented — see the 2026-06-17 progress log.)

## What ASS is

Analyst's Statistical Suite (ASS) is an open-source, SAS-compatible data processing and analytics engine written primarily in **Go** and driven from the command line. The goal is **behavioral compatibility** with a useful subset of SAS programs (DATA step, PROC SORT/PRINT/SQL, import/export, formats, macro basics) via clean-room implementation — not a full SAS clone. ETL and reporting compatibility come before advanced statistical procedures.

Legal constraint: implement from public examples and observed behavior only. Do not copy proprietary SAS documentation, source, branding, or non-public implementation details.

## Architecture

The execution pipeline is a classic interpreter chain:

```
SAS source → Lexer → Macro preprocessor → Parser → AST → IR → Runtime/VM → Tables, reports, logs, output files
```

Planned package layout (from the design doc):

- `cmd/ass/` — CLI entry point (`ass file.sas`, `ass test corpus/`)
- `lexer/` — tokenizer (comments, strings, identifiers, numbers, semicolons; detects DATA/PROC steps)
- `macro/` — macro preprocessor; runs **before** the parser (`%let`, `&var` expansion, `%macro`/`%mend`)
- `parser/` + `ast/` — SAS parser producing the syntax tree
- `runtime/` — DATA step runtime (the core of the system)
- `vm/` — optional bytecode VM
- `table/` — dataset abstraction (library/name, columns, types, labels, formats, informats, rows, missing values, metadata)
- `proc/` — PROC implementations
- `formats/` — formats and informats
- `sql/` — PROC SQL bridge or engine (may be backed by DuckDB/SQLite/PostgreSQL or a Go-native engine)
- `dbio/` — external-database LIBNAME engines (`table.Backend` over `database/sql`); read-only Postgres/SQL Server/Oracle via pure-Go drivers. See `docs/databases.md`
- `log/` — SAS-style logging
- `corpus/` + `tests/` — compatibility corpus and tests

### Execution model — critical concepts

These behaviors define what "SAS-compatible" means here; get them right before adding breadth:

- **Step-at-a-time execution.** A source file is a sequence of DATA and PROC steps delimited by `run;`/`quit;`. Each step is independently parsed, compiled to IR, then executed. Steps share data only through named datasets.
- **Program Data Vector (PDV) + implicit row loop.** The DATA step runtime models SAS's PDV: each iteration of the implicit loop updates variables, applies statements top-to-bottom, and outputs rows per SAS rules. Automatic variables `_N_` and `_ERROR_`, missing-value semantics, and the distinction between character (`$`) and numeric variables all live here.
- **Compatibility levels** (the roadmap): L0 parse-only → L1 basic DATA step → L2 core PROCs (print/sort/contents/import/export) → L3 PROC SQL/ETL → L4 macro basics → L5 advanced DATA step (`retain`, arrays, BY-groups, `first.`/`last.`, `merge`, `in=`, user formats) → L6 statistical PROCs. Build and test in this order.

## Compatibility test harness

Correctness is measured against a tagged corpus, not just unit tests. The harness (`ass test`) is a first-class deliverable, with planned modes:

```bash
ass test corpus/                 # full run
ass test --parse-only corpus/    # parse without executing
ass test --feature data-step     # filter by feature tag
```

**Compatibility is measured at the level of VALUES, not byte-identical presentation.** ASS targets value/result compatibility (same dataset columns/values, same SQL result sets, same computed statistics) — the SAS data semantics are deterministic, so expected values are hand-derivable and verifiable without a SAS license. Byte-for-byte identical PROC listings/log wording versus real SAS are an explicit non-goal. See `docs/COMPATIBILITY.md` ("What compatibility means") and `corpus/README.md`.

Each corpus item carries YAML metadata (`id`, `source`, `license`, `features`, `expected.parse/execute/output`, `priority`) plus the primary correctness check `expected.datasets` (hand-derived expected values per output dataset). Each run reports parsed / executed / passed and a value-verified count, rolling up to per-feature percentages. When implementing a feature, add a corpus item tagged with that feature and assert its output values via `expected.datasets` rather than only Go unit tests.

## Build & test commands

```bash
go build ./...
go test ./...
go test ./lexer/                 # single package
go test ./runtime/ -run TestEval # single test by name
go run ./cmd/ass run file.sas    # execute a SAS program
go run ./cmd/ass parse file.sas  # dump the AST
go run ./cmd/ass tokens file.sas # dump the token stream
```

**CGo is required.** PROC SQL embeds SQLite via `github.com/mattn/go-sqlite3`, so building needs `CGO_ENABLED=1` (the default) and a C compiler (e.g. gcc). See `sql/DECISION.md` for the rationale and consequences.

## Guiding principle

The DATA step runtime is the sun; everything else orbits it. Grow one tested, corpus-backed feature at a time rather than scaffolding broad-but-broken coverage.

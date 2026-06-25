# Analyst’s Statistical Suite

## Initial Design and Compatibility Plan

## 1. Purpose

Analyst’s Statistical Suite, abbreviated ASS, is an open-source SAS-compatible data processing and analytics system.

The first goal is not to replace every SAS product. The first goal is to run a useful subset of common SAS programs, especially DATA step, PROC SORT, PROC PRINT, PROC SQL, import/export, formats, and macro basics.

The project should prioritize real-world ETL and reporting compatibility before advanced statistical procedures.

## 2. Design Goals

ASS should be:

* Open source
* Cross-platform
* Written primarily in Go
* Usable from the command line
* Compatible with common `.sas` source files where practical
* Test-driven against a public SAS code corpus
* Designed around clear compatibility levels

## 3. Compatibility Levels

### Level 0: Parser Recognition

The system can tokenize and parse SAS-like files, identify DATA steps, PROC blocks, comments, statements, and macro directives.

### Level 1: Basic DATA Step

Supports:

* `data`
* `set`
* `input`
* `datalines`
* assignments
* `if/then`
* `do/end`
* implicit row loop
* `_N_`
* `_ERROR_`
* missing values
* character and numeric variables

### Level 2: Core Procedures

Supports:

* `proc print`
* `proc sort`
* basic `proc contents`
* basic `proc import`
* basic `proc export`

### Level 3: SQL and ETL

Supports:

* `proc sql`
* `create table as select`
* joins
* where clauses
* group by
* order by
* calculated columns
* table aliases

This may be backed internally by DuckDB, SQLite, PostgreSQL, or a Go-native execution engine.

### Level 4: Macro Basics

Supports:

* `%let`
* macro variables
* `&var` expansion
* `%macro` / `%mend`
* positional and named macro parameters
* basic `%if/%then/%else`
* basic `%do`

### Level 5: Advanced DATA Step Compatibility

Supports:

* `retain`
* arrays
* BY-group processing
* `first.` and `last.` variables
* `merge`
* `in=`
* formats and informats
* date literals
* user-defined formats

### Level 6: Statistical Procedures

Later support for selected procedures:

* `proc means`
* `proc freq`
* `proc summary`
* `proc reg`
* `proc glm`

These should be added after the DATA step and ETL core are strong.

## 4. Architecture

```text
SAS source
  ↓
Lexer
  ↓
Macro preprocessor
  ↓
Parser
  ↓
AST
  ↓
Intermediate representation
  ↓
Runtime engine / VM
  ↓
Tables, reports, logs, output files
```

## 5. Major Components

```text
ass/
  cmd/ass/              Command-line interface
  lexer/                Tokenizer
  parser/               SAS parser
  ast/                  Syntax tree definitions
  macro/                Macro preprocessor
  runtime/              DATA step runtime
  vm/                   Optional bytecode VM
  table/                Dataset abstraction
  proc/                 PROC implementations
  formats/              Formats and informats
  sql/                  PROC SQL bridge or engine
  log/                  SAS-style logging
  corpus/               Test corpus metadata
  tests/                Unit and compatibility tests
```

## 6. Execution Model

ASS should behave like SAS at the step level.

A source file is processed as a sequence of steps:

```sas
data example;
  ...
run;

proc print data=example;
run;
```

Each DATA or PROC block is parsed, compiled to an internal representation, then executed.

The DATA step runtime should model the SAS Program Data Vector concept. Each row iteration updates variables, applies statements, and outputs rows according to SAS-like rules.

## 7. Dataset Model

Datasets should support:

* library name
* dataset name
* columns
* column types
* labels
* formats
* informats
* rows
* missing values
* metadata

Initial storage may be in-memory. Later storage may include:

* local ASS dataset files
* CSV
* Parquet
* DuckDB
* SQLite
* PostgreSQL

## 8. Test Corpus Plan

The project should build a public compatibility corpus from several legal, public sources.

Initial sources:

1. SAS official code examples repository
2. SAS Support DATA step samples
3. SAS Communities code examples
4. Public SAS macro repositories
5. Small hand-written regression tests
6. User-contributed real-world examples, with proprietary data removed

Each corpus item should have metadata:

```yaml
id: data_step_basic_001
source: sas-code-examples
license: upstream license
features:
  - data-step
  - datalines
  - proc-print
expected:
  parse: pass
  execute: pass
  output: normalized-table
priority: 1
```

## 9. Corpus Classification

Examples should be classified by feature:

```text
DATA step basics
DATA step input/datalines
Assignments and expressions
IF/THEN/ELSE
DO loops
SET
MERGE
BY groups
Formats
Informats
Dates
PROC PRINT
PROC SORT
PROC SQL
PROC IMPORT
PROC EXPORT
Macros
Statistical PROCs
ODS/output
Graphics
Unsupported/vendor-specific
```

## 10. Compatibility Harness

The test harness should support several modes:

```bash
ass test corpus/
ass test --parse-only corpus/
ass test --feature data-step
ass test --feature proc-sort
ass test --compare-output expected/
```

Each test should report:

```text
parsed: yes/no
executed: yes/no
log compatible: yes/no
output compatible: yes/no
unsupported features: list
```

The project should publish compatibility percentages:

```text
DATA step basics:     84%
PROC PRINT:          100%
PROC SORT:            92%
PROC SQL:             41%
Macro language:        8%
Overall corpus:       37%
```

## 11. Legal and Licensing Notes

ASS should avoid copying proprietary SAS documentation, source code, examples without compatible licenses, branding, or internal implementation details.

The project should aim for behavioral compatibility based on public examples, user tests, and clean-room implementation.

The name should clearly state that ASS is not affiliated with SAS Institute.

Suggested wording:

> Analyst’s Statistical Suite is an independent open-source project. It is not affiliated with, endorsed by, or sponsored by SAS Institute Inc.

## 12. Initial Milestones

### Milestone 1: CLI and Lexer

* `ass file.sas`
* tokenize source
* recognize comments, strings, identifiers, numbers, semicolons
* detect DATA and PROC steps

### Milestone 2: Parser

* parse DATA step blocks
* parse PROC blocks
* produce AST
* parse simple expressions

### Milestone 3: Minimal DATA Step Runtime

Support:

```sas
data people;
  input name $ age;
  datalines;
John 25
Mary 30
;
run;
```

### Milestone 4: PROC PRINT

Support:

```sas
proc print data=people;
run;
```

### Milestone 5: Expressions and Filtering

Support:

```sas
data adults;
  set people;
  if age >= 18;
run;
```

### Milestone 6: PROC SORT

Support:

```sas
proc sort data=people;
  by age;
run;
```

### Milestone 7: Basic PROC SQL

Support:

```sas
proc sql;
  create table adults as
  select *
  from people
  where age >= 18;
quit;
```

### Milestone 8: Corpus Harness

* import public examples
* tag by feature
* run parse tests
* run execution tests
* produce compatibility report

## 13. Guiding Principle

The project should not begin as a complete SAS clone.

It should begin as a practical SAS-compatible ETL engine, then grow one tested feature at a time.

The heart of the system is the DATA step runtime. Everything else can orbit that sun.

## 14. Architecture Decision: an interpreter, not a native compiler

**Decision (2026-06-23):** ASS commits to the *interpreter family* as its permanent
execution architecture — compile each step to an internal representation, then
execute it — and treats native machine-code generation (LLVM/Cranelift/transpile-to-C)
as **off the roadmap**, not as an eventual destination.

### The real axis is the execution model, not "interpreter vs. compiler"

Performance for this class of tool lives on a spectrum, and nearly every useful
point on it is still an interpreter:

1. **Scalar tree-walk** — execute over the AST (the current runtime).
2. **Scalar bytecode VM** — compile expressions/statements to bytecode and
   interpret that (the planned `vm/` package). Big win over pointer-chasing the
   AST; stays pure Go, one semantics codebase.
3. **Vectorized / columnar execution** — process batches of columns rather than
   rows (the model behind DuckDB, Polars, Velox). Also an interpreter.

Native codegen is a separate, rarely-justified path *off* this spectrum. The
fastest analytical engines in the world are interpreters (vectorized ones), and
SAS itself compiles the DATA step to an internal machine and interprets it rather
than emitting native code. ASS matches that family by design.

### Why

- **Portability.** A single pure-Go static binary runs on every architecture.
  This is validated, not assumed: the engine builds and passes the full corpus
  and test suite on big-endian **s390x/LinuxONE** (`CGO_ENABLED=0`, a fully
  static `ELF … MSB executable`). Native codegen would forfeit this.
- **Correctness.** One execution path means one place for SAS semantics (missing
  values, type coercion, formats) to live. Behavioral compatibility is the whole
  value proposition; a second compiled path would double the surface where
  behavior can silently drift from SAS.
- **Velocity & maintenance.** Features ship once; no LLVM/Cranelift dependency,
  no per-platform codegen bugs, no JIT warm-up or sandboxing surface.

### Conditions that would (and would not) justify moving up a rung

- Build the **bytecode VM** only when profiling of a *real, large* workload shows
  the per-row interpreter loop — not I/O, not a PROC, not the SQL engine — is the
  bottleneck. Cheap interpreter optimizations come first (resolve variable names
  to PDV **slot indices** at compile time, flatten the statement list, cut
  per-row allocations); these often recover most of the gap with no architecture
  change. **Empirical update (2026-06-24, [`perf.md`](perf.md)):** the per-row
  loop is indeed the bottleneck, but the measured cost is *data representation*
  (per-row `map[string]Value` allocation + PDV string-map hashing), not opcode
  dispatch — so the VM is deferred and the **slot-indexed PDV + non-map rows** is
  the next perf investment (it also becomes the VM's foundation if one is ever
  built).
- Consider **vectorized execution** only if the product pivots toward being a
  general high-performance analytical engine. Even then the answer stays inside
  the interpreter family.
- Whatever is added, the existing engine remains the **reference oracle**: run the
  value-verified corpus through both and assert identical results (differential
  testing). The corpus is what makes a faster engine *safe* to add — so corpus
  breadth is the real prerequisite for this work.

## 15. Eliminating the C-compiler requirement

The only hard CGo dependency is the embedded SQLite used by PROC SQL and the
`sqlite` LIBNAME engine (`mattn/go-sqlite3`). It is already gated behind the `cgo`
build constraint, so `CGO_ENABLED=0` yields a pure-Go static binary today — minus
PROC SQL. Two end-states remove CGo from the *default* build:

1. **Swap to `modernc.org/sqlite`** (pure-Go, transpiled SQLite, used through
   `database/sql`). This makes the default build CGo-free while keeping PROC SQL.
   The one risk was big-endian correctness — **verified resolved (2026-06-23):**
   `modernc.org/sqlite v1.53.0` passes a focused big-endian test on `linux/s390x`
   (bit-exact float round-trip, an 8-byte integer byte-order canary, SUM/AVG/MIN/
   MAX, and `ORDER BY REAL`). This swap is independent of the VM and can be done
   at any time. **Done (2026-06-23):** `mattn/go-sqlite3` was replaced wholesale by
   `modernc.org/sqlite`; the `cgo` build gating, the PROC SQL pure-Go stub, and the
   corpus skip logic were removed, so PROC SQL is now always available and the
   default `go build` (even `CGO_ENABLED=0`) is fully static. See `sql/DECISION.md`.
2. **A native ASS SQL executor on the shared execution core** (far future). Drops
   SQLite entirely and unifies SQL semantics with the DATA step. Large; this is
   the only piece that genuinely benefits from the bytecode VM existing first.

DB2 (`-tags db2`, IBM CLI driver) is intentionally always opt-in CGo and excluded
from the static binary by construction.

## 16. Interactivity and notebooks

Notebook/REPL use reinforces the interpreter decision: a Jupyter kernel needs
incremental execution, **live session state across cells** (the WORK library,
macro symbol table, variable definitions persisting between submissions), and
introspection for explorer panels — all natural for a resident interpreter.

The keystone is a **resident session model**: turning the batch runner
("parse file → run → exit") into a long-lived interpreter that holds the library
and symbol tables and accepts successive program fragments. This is *not* gated on
the bytecode VM, and it also unlocks the interactive AI assistant described in
`future-directions.md`. The Jupyter wire protocol is ZeroMQ, and a pure-Go
implementation (`go-zeromq/zmq4`) exists, so an ASS kernel does **not** reintroduce
a C compiler.

**Done (2026-06-24):** the resident session model is implemented as the
`session` package — `session.New()` holds a persistent `table.Library` +
`macro.Processor`, and `(*Session).Submit(src, logger)` macro-expands → parses →
runs one fragment against that shared state (datasets, librefs, macro vars, and
macro defs all carry across submissions). The batch runner (`ass file.sas`) is
now literally one `Submit` on a fresh session, and `ass repl` is the first
interactive consumer. The Jupyter kernel is the next consumer of the same API.

### Dependency map (what needs what)

- **Resident session model** — keystone; enables notebooks and interactive AI;
  needs no performance work.
- **`modernc.org/sqlite` swap** — independent; verified on big-endian; removes CGo
  from the default build.
- **Jupyter kernel** — **done 2026-06-24**: the `kernel` package implements the
  Jupyter wire protocol (v5.3, HMAC-signed) over pure-Go `zmq4`, feeding each
  cell to a `session.Session`. `ass kernel --install` registers the kernelspec;
  `ass kernel <conn>` is the launch form. Builds CGo-free. See
  [`jupyter.md`](jupyter.md). A real in-process ZeroMQ wire test
  (`kernel/kernel_test.go`) drives kernel_info → execute (with streamed output)
  → shutdown. Tabular PROCs (PRINT/MEANS/FREQ one-way/SQL/REG) render as **HTML
  tables** via `display_data` (rich sink on `log.Logger`; inert outside the
  kernel, so batch/REPL stay byte-identical). Forward-looking: HTML for the
  remaining text output, completion/inspection, cooperative interrupt.
- **Bytecode VM** — performance; gated on profiling evidence.
- **Own SQL engine** — far future; the one item that wants the VM done first.

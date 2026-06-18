# Contributing to ASS

ASS grows one tested, corpus-backed feature at a time. The DATA step runtime is
the core; everything else orbits it. Please read [`design.md`](design.md) (rationale),
[`../CLAUDE.md`](../CLAUDE.md) (architecture), and [`PLAN.md`](PLAN.md) (development log) before starting.

## Prerequisites

- Go (see `go.mod` for the version) and a C compiler — **CGo is required** for
  the embedded SQLite used by PROC SQL (`CGO_ENABLED=1`, the default).

```bash
go build ./...
go test ./...
go vet ./...
```

## The compatibility corpus is the source of truth

Correctness is measured against the corpus in `corpus/`, not only Go unit tests.
When you add or change a feature, add a corpus item tagged with that feature.

### Adding a corpus item

Create `corpus/<id>/` with two files:

- `input.sas` — a small, focused SAS program.
- `meta.yaml`:

```yaml
id: my_feature_001
source: hand-written          # or a clearly-licensed public example
license: MIT
features:
  - data-step
  - my-feature                # tags from corpus/FEATURES.md
expected:
  parse: pass                 # pass | fail
  execute: pass               # pass | fail | skip
  output: unverified          # verified | unverified | none
priority: 1
notes: |
  One paragraph describing what it exercises and the expected result.
```

If you introduce a new feature tag, add it to `corpus/FEATURES.md`.

Run it:

```bash
ass test corpus/                 # full run
ass test --feature my-feature corpus/
ass test --parse-only corpus/
ass test -v corpus/              # show failure detail
```

The harness exits non-zero if any item regresses.

## Where things live

```
cmd/ass/   CLI entry point and command dispatch
lexer/     tokenizer (+ source spans, datalines mode)
macro/     macro preprocessor (runs before the parser)
parser/    + ast/  recursive-descent + Pratt expression parser
runtime/   DATA step engine: PDV, implicit loop, eval, merge, by-groups
table/     dataset/value/library model (Value.Compare is the SAS ordering)
proc/      PROC implementations behind a registry (print, sort, sql, means, freq)
sql/       PROC SQL engine (embedded SQLite)
formats/   formats/informats and date handling
log/       SAS-style NOTE/WARNING/ERROR logging
corpus/    compatibility corpus + FEATURES.md tag catalog
```

## Adding a PROC

1. Create `proc/<name>.go` with a type implementing the `Proc` interface
   (`Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error`).
2. `Register("<name>", yourProc{})` from an `init()`.
3. Read options from `step.Data`/`step.Options` and statements from `step.Body`
   (add a statement AST node + parser case if you need a new one).
4. Reuse `renderListing` (in `proc/print.go`) to print tabular output.
5. Add a corpus item and a Go unit test (test the pure result-building function,
   not stdout, where possible).

## Adding a DATA step function

Add a `case` in `runtime/functions.go` (`evalCall`) and a table-test entry in
`runtime/eval_test.go`. Follow SAS missing-value rules: aggregate functions
ignore missing; scalar functions propagate it.

## Conventions

- Keep changes scoped to one feature; leave the tree green (`go build` +
  `go test` pass) at every commit.
- Match the surrounding code's style and comment density.
- Clean-room only: implement from public examples and observed behavior. Do not
  copy proprietary SAS documentation, source, or non-public details.
- Update `PLAN.md`'s progress log with what changed, key files, and any
  deviations or deferrals.

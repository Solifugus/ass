# ASS Development Plan

Living plan for building Analyst's Statistical Suite (ASS), a SAS-compatible ETL/analytics engine in Go. See `ass-design.md` for the design rationale and `CLAUDE.md` for architecture notes.

---

## How to resume (read this first)

If you are a fresh Claude Code instance with no memory of prior work:

1. Read this file top to bottom. The **Progress log** (bottom) is the source of truth for what's done.
2. Find the first step whose checkbox is `[ ]` (unchecked) — that is the next step. Steps marked `[x]` are done; `[~]` means in progress (check the Progress log for partial notes).
3. Verify reality before trusting checkboxes: run `go build ./...` and `go test ./...`, and skim the relevant files. If a step is marked done but its acceptance criteria fail, fix that before moving on.
4. Do the step. Keep changes scoped to that step only.
5. When done: change its `[ ]` to `[x]`, append a dated entry to the **Progress log** (what you did, key files, any deviations or decisions), and stop or continue per the user's instruction.

**Rules for keeping steps context-safe:**
- Each step is sized to fit comfortably in one context window. If a step feels too big, split it into sub-steps in place and note that in the Progress log.
- Never start a step without first updating its status to `[~]` if you might not finish it in one session.
- Prefer leaving the tree green (`go build` + `go test` pass) at the end of every step.

**Status legend:** `[ ]` not started · `[~]` in progress · `[x]` done · `[!]` blocked (explain in Progress log)

---

## Phase 0 — Project scaffolding

- [x] **0.1 Initialize Go module.** Run `go mod init github.com/<owner>/ass` (confirm the module path with the user; default to `ass` if unknown). Create `.gitignore` for Go. Add a top-level `README.md` stub with the project description and the "not affiliated with SAS Institute" disclaimer from the design doc §11. Acceptance: `go build ./...` succeeds (no packages yet is fine).
- [x] **0.2 Create package skeleton.** Create empty packages with a single `doc.go` (package comment) each: `cmd/ass`, `lexer`, `parser`, `ast`, `macro`, `runtime`, `table`, `proc`, `formats`, `sql`, `log`, `corpus`. Do NOT create `vm` yet (optional, deferred). Acceptance: `go build ./...` succeeds.
- [x] **0.3 Minimal CLI entry point.** In `cmd/ass/main.go`, implement arg parsing for `ass <file.sas>` (read file, print token count placeholder) and `ass test <dir>` (stub that prints "not implemented"). Wire nothing else yet. Acceptance: `go run ./cmd/ass --help` prints usage; `go run ./cmd/ass somefile.sas` reads the file without crashing.
- [x] **0.4 Git init & first commit.** `git init`, commit the scaffold. (Only if the user wants version control — confirm first.) Acceptance: clean `git status`.

## Phase 1 — Research & sample gathering

- [x] **1.1 Catalog SAS language constructs to target.** Write `corpus/FEATURES.md` listing the feature tags from design doc §9 (DATA step basics, input/datalines, assignments, IF/THEN/ELSE, DO loops, SET, MERGE, BY groups, formats, informats, dates, PROC PRINT/SORT/SQL/IMPORT/EXPORT, macros, statistical PROCs, ODS, unsupported). For each, one line on what it means. This is the canonical tag list. Acceptance: file exists, tags match design doc.
- [x] **1.2 Define the corpus item format.** Write `corpus/README.md` specifying the on-disk layout: each item is a directory `corpus/<id>/` containing `input.sas`, `meta.yaml` (schema from design doc §8: id, source, license, features, expected.parse/execute/output, priority), and optional `expected_output.txt` / `expected_log.txt`. Acceptance: format documented with one worked example.
- [x] **1.3 Gather/author Level-1 samples.** Create 5–8 small, hand-written corpus items covering: data+input+datalines, simple assignment, if/then subsetting, do loop, _N_ usage. Keep each tiny. Use only hand-written or clearly-licensed public examples (record license in meta.yaml). Acceptance: items parse-valid by inspection; `corpus/` has the new dirs with meta.yaml each.
- [x] **1.4 Gather Level-2 samples.** Add 4–6 items for PROC PRINT and PROC SORT (with/without BY). Acceptance: dirs + meta.yaml present.
- [x] **1.5 Gather Level-3/4 samples.** Add 3–5 PROC SQL items and 3–5 macro items (%let, &var, %macro). Mark these `priority: 2`. Acceptance: dirs + meta.yaml present.
- [x] **1.6 Capture expected outputs.** NOTE (revised during Phase 1): PROC PRINT's exact column spacing is defined by the renderer in Phase 5.2, so hand-deriving `expected_output.txt` now would bake in arbitrary spacing. All items are currently marked `output: unverified` in meta.yaml with the expected *content* described in each item's `notes:`. **Backfill `expected_output.txt` and flip `output: verified` in Phase 5.2** once the PROC PRINT renderer format is locked (the renderer must match these, test-driven). Acceptance (revised): every item's expected result is documented in its `notes:`; output verification deferred to 5.2.

## Phase 2 — Lexer

- [x] **2.1 Token types.** In `lexer/token.go`, define `TokenType` constants and a `Token` struct (type, literal, line, col). Cover: identifiers, numbers, string literals (single+double quote), `;`, `=`, comparison/arith operators, `(` `)` `,` `.` `&` `%`, comment markers (`*...;` and `/* */`), and keyword detection helper. Acceptance: compiles; constants documented.
- [x] **2.2 Core scanner.** In `lexer/lexer.go`, implement a `Lexer` that scans a string into tokens: whitespace skipping, line/col tracking, identifiers, numbers, strings, single-char tokens. Add `lexer/lexer_test.go` with table tests. Acceptance: `go test ./lexer/` passes for basic tokens.
- [x] **2.3 Comments & SAS quirks.** Handle SAS's two comment forms (`* ... ;` statement comments and `/* ... */` block comments) and the `$` character-variable marker. Add tests. Acceptance: comment tests pass; comments produce no spurious tokens.
- [x] **2.4 Step/keyword recognition.** Recognize statement-leading keywords enough to detect `data`, `proc`, `run`, `quit`, `datalines`/`cards`, and handle the special raw-data region after `datalines;` up to a line with `;`. Add tests for a full `data ... datalines ... run;` token stream. Acceptance: datalines region tokenized correctly (raw lines, not re-lexed as code).
- [x] **2.5 Wire lexer into CLI.** Make `ass <file.sas>` print the token stream (debug mode) so the lexer is exercisable end-to-end. Acceptance: running it on a Level-1 corpus item prints sensible tokens.

## Phase 3 — Parser & AST

- [ ] **3.1 AST node definitions.** In `ast/`, define interfaces (`Node`, `Statement`, `Expression`) and the program/step containers: `Program` (list of steps), `DataStep`, `ProcStep` (generic with proc name + raw options/statements for now). Acceptance: compiles.
- [ ] **3.2 Statement & expression nodes.** Add nodes for: assignment, `set`, `input` (with var list + `$` flags), `datalines` (raw block), `if/then/else`, `do/end`, `output`, `keep/drop`, plus expression nodes (literal, identifier, binary op, unary op, function call). Acceptance: compiles; nodes have `String()` for debugging.
- [ ] **3.3 Program/step parser.** In `parser/parser.go`, implement top-level parsing that splits the token stream into steps by `data`/`proc` ... `run;`/`quit;` and dispatches to step parsers. Add `parser/parser_test.go`. Acceptance: parses a multi-step file into a `Program` with the right step count/types.
- [ ] **3.4 Expression parser (Pratt).** Implement precedence-climbing/Pratt parsing for SAS expressions: arithmetic (`+ - * / **`), comparison (`= ^= < <= > >=` and word forms `eq ne lt`...), logical (`and or not`), function calls, parentheses. Add tests. Acceptance: expression tests pass including precedence.
- [ ] **3.5 DATA step statement parser.** Parse the bodies of DATA steps into the statement nodes from 3.2. Add tests against Level-1 corpus items. Acceptance: Level-1 items parse to expected AST shape.
- [ ] **3.6 PROC step option parser.** Parse common PROC option syntax: `proc <name> data=<ds> (options);` and statement lines like `by`, `var`, `where`. Keep proc-specific semantics out (just structure). **Caveat from Phase 2:** the lexer emits `data` as the `DATA` keyword token even in `proc print data=people` (SAS keywords are contextual). The option parser must accept the `DATA` token (and likely `SET`, etc.) where an option name is expected, rather than only `IDENT`. Acceptance: PROC PRINT/SORT items parse.
- [ ] **3.7 Wire parser into CLI.** Add `ass parse <file.sas>` (or a flag) that prints the AST. Acceptance: prints AST for corpus items.

## Phase 4 — DATA step runtime (the core)

- [ ] **4.1 Dataset model.** In `table/`, implement `Dataset` (library name, name, ordered columns with name/type/label/format/informat, rows as `[]Row`), `Column`, and a `Value` type supporting numeric, character, and **missing** (`.` numeric, `''` char). Add tests for construction and missing-value semantics. Acceptance: `go test ./table/` passes.
- [ ] **4.2 In-memory library/catalog.** Implement a `Library` map (e.g. `work`) holding datasets by name, with get/put. This is how steps pass data. Acceptance: tests for put/get and overwrite.
- [ ] **4.3 PDV + expression evaluator.** In `runtime/`, implement the Program Data Vector (variable name → current Value) and an expression evaluator over the PDV (numbers, strings, missing propagation, arithmetic, comparison returning 1/0, logical ops, a few core functions like `sum`, `upcase`). Add tests. Acceptance: evaluator tests pass including missing-value propagation rules.
- [ ] **4.4 Implicit loop & assignment/output.** Implement the DATA step driver: initialize PDV, run the implicit row loop, execute assignment statements, handle automatic `_N_` (iteration counter) and `_ERROR_`. For a step with no input, run once. Implement explicit/implicit `output`. Acceptance: a `data` step with assignments produces the expected dataset.
- [ ] **4.5 `input` + `datalines`.** Implement reading the raw datalines region per the `input` spec (list input: space-delimited, `$` = char). Populate the PDV and output one row per data line. Acceptance: the canonical `data people; input name $ age; datalines; ... run;` produces a 2-row dataset.
- [ ] **4.6 `set` (read existing dataset).** Implement `set <ds>`: the implicit loop iterates rows of the input dataset into the PDV. Acceptance: `data b; set a; run;` copies dataset a to b.
- [ ] **4.7 `if/then/else` + subsetting if.** Implement conditional execution and the subsetting-`if` (a bare `if cond;` that drops the row when false). Acceptance: `data adults; set people; if age>=18; run;` filters correctly.
- [ ] **4.8 `do/end` loops & `keep`/`drop`.** Implement iterative `do`/`do while`/`do until` and column selection via `keep`/`drop` statements (and dataset options if feasible). Acceptance: tests for a do loop and keep/drop pass.
- [ ] **4.9 SAS-style log output.** In `log/`, implement a logger that writes SAS-like NOTEs (e.g. "NOTE: The data set WORK.PEOPLE has N observations and M variables."). Wire DATA step to emit these. Acceptance: log lines match the expected format for Level-1 items.

## Phase 5 — PROC PRINT

- [ ] **5.1 PROC dispatch.** In `proc/`, define a `Proc` interface (run against a Library + step AST + logger) and a registry mapping proc name → implementation. Wire the runtime to dispatch PROC steps. Acceptance: unknown procs produce a clean "not supported" log note, not a crash.
- [ ] **5.2 PROC PRINT core.** Implement `proc print data=<ds>;` rendering a SAS-like listing (Obs column + variables, right/left alignment by type). Support `var` to select/order columns. Add tests comparing rendered text to expected. **Also backfill the deferred corpus `expected_output.txt` files here (see step 1.6): once the renderer format is locked, write expected output for the Level-1/Level-2 items and flip their `output: unverified` → `verified`.** Acceptance: PROC PRINT corpus items match expected output.
- [ ] **5.3 PROC PRINT options.** Add `noobs`, `label` (use column labels as headers). Acceptance: tests for noobs/label pass.

## Phase 6 — Expressions, functions & filtering polish

- [ ] **6.1 Expand function library.** Add commonly-used DATA step functions: `substr`, `trim`, `left`, `length`, `scan`, `index`, `int`, `round`, `abs`, `min`, `max`, `mean`/`sum` (varargs). Table-test each. Acceptance: function tests pass.
- [ ] **6.2 `where` vs subsetting if.** Ensure `where` clauses (DATA step option and statement) filter correctly and document the difference from subsetting `if`. Acceptance: where tests pass.
- [ ] **6.3 Type coercion & formatting basics.** Implement automatic numeric↔character coercion rules and default numeric printing (BEST. format approximation). Acceptance: coercion tests pass.

## Phase 7 — PROC SORT

- [ ] **7.1 PROC SORT core.** Implement `proc sort data=<ds>; by <vars>;` with stable multi-key sort, ascending default and `descending` per key. Support `out=` for a separate output dataset (in place otherwise). Acceptance: SORT corpus items produce expected order.
- [ ] **7.2 `nodupkey`/`dupout`.** Add duplicate removal by key. Acceptance: tests pass.
- [ ] **7.3 BY-group plumbing.** Compute `first.`/`last.` flags for BY variables and expose them to the DATA step runtime (sets up Phase 10). Acceptance: first./last. computed correctly on a sorted dataset (unit-tested in table/runtime).

## Phase 8 — PROC SQL (Level 3)

- [ ] **8.1 Decide the SQL backend.** Evaluate: Go-native mini-engine vs embedding SQLite/DuckDB. Write a short decision note in `sql/DECISION.md` (record the trade-off and the choice; default recommendation: start Go-native for `select` over in-memory datasets to avoid a heavy dependency, revisit later). Acceptance: decision recorded. **Confirm the choice with the user before implementing.**
- [ ] **8.2 SQL lexer/parser (or bridge).** Per 8.1: either a focused SQL parser for `select`/`from`/`where`/`group by`/`order by`/`create table as`, or a translation layer to the chosen embedded engine. Acceptance: parses the SQL corpus items.
- [ ] **8.3 SELECT execution.** Implement projection, `where`, calculated columns, table aliases over in-memory datasets. Acceptance: simple select items produce expected results.
- [ ] **8.4 Joins & grouping.** Implement inner/left/right joins, `group by` with aggregates (count/sum/avg/min/max), `order by`. Acceptance: join and group-by items pass.
- [ ] **8.5 `create table as`.** Materialize query results into the library. Acceptance: `create table x as select ...` then PROC PRINT of x works end-to-end.

## Phase 9 — Macro basics (Level 4)

- [ ] **9.1 Macro variable store & `%let`.** In `macro/`, implement a symbol table and `%let name = value;`. Acceptance: store/retrieve tests pass.
- [ ] **9.2 `&var` resolution.** Implement macro-variable reference expansion in source text (including `&&` and `.`-terminated refs) as a preprocessing pass before lexing. Acceptance: expansion tests pass, including nested refs.
- [ ] **9.3 `%macro`/`%mend` + parameters.** Implement macro definition/invocation with positional and named parameters. Acceptance: a parameterized macro expands correctly.
- [ ] **9.4 `%if/%then/%else` and `%do`.** Implement basic macro control flow during expansion. Acceptance: conditional/loop macro tests pass.
- [ ] **9.5 Integrate preprocessor into pipeline.** Ensure macro expansion runs before the lexer in the CLI/runtime flow (matches the architecture diagram). Acceptance: a corpus item using macros runs end-to-end.

## Phase 10 — Advanced DATA step (Level 5)

- [ ] **10.1 `retain`.** Variables keep values across iterations. Acceptance: retain test (running total) passes.
- [ ] **10.2 Arrays.** Implement `array` declaration and subscripted references. Acceptance: array test passes.
- [ ] **10.3 BY-group processing in DATA step.** Use `first.`/`last.` (from 7.3) inside the implicit loop. Acceptance: by-group aggregation test passes.
- [ ] **10.4 `merge` + `in=`.** Implement match-merge by BY variables with `in=` dataset flags. Acceptance: merge test passes.
- [ ] **10.5 Formats & informats.** Implement `formats/` core formats (e.g. numeric `w.d`, `dollar`, `date`/`datetime`) and informats; apply on input and on PRINT. Acceptance: format application tests pass.
- [ ] **10.6 Date literals & user formats.** Support `'01JAN2020'd` date literals and `proc format` user-defined formats. Acceptance: date + user-format tests pass.

## Phase 11 — Compatibility harness

- [ ] **11.1 Corpus loader.** In `corpus/`, implement loading items from disk (parse `meta.yaml`, read input/expected files). Acceptance: loader reads all existing corpus items into structs.
- [ ] **11.2 `ass test` runner.** Implement `ass test <dir>`: for each item, run parse and (if expected) execute, compare output/log, collect pass/fail + unsupported features. Acceptance: runner produces a per-item report.
- [ ] **11.3 Filters & modes.** Add `--parse-only`, `--feature <tag>`, `--compare-output <dir>`. Acceptance: each flag changes behavior as documented in design §10.
- [ ] **11.4 Compatibility report.** Aggregate per-feature and overall compatibility percentages and print the summary table (design §10). Acceptance: running the full corpus prints the percentage table.
- [ ] **11.5 CI-friendly exit codes.** Non-zero exit on regressions; optional JSON output for tooling. Acceptance: exit code reflects failures.

## Phase 12 — Statistical procedures (Level 6, later)

- [ ] **12.1 PROC MEANS / SUMMARY.** Descriptive stats (n, mean, std, min, max) with `class`/`by`. Acceptance: means corpus items pass.
- [ ] **12.2 PROC FREQ.** One- and two-way frequency tables. Acceptance: freq items pass.
- [ ] **12.3 PROC REG / GLM (stretch).** Basic linear regression. Acceptance: reg item produces coefficients within tolerance. (Confirm scope with user — large effort.)

## Phase 13 — Final documentation & release

- [ ] **13.1 Update CLAUDE.md.** Replace the "Project status: only design doc" framing with the real build/test commands, actual package responsibilities, and any architecture deviations. Acceptance: CLAUDE.md matches the code.
- [ ] **13.2 User README & CLI docs.** Document installation, `ass` usage, supported features per compatibility level, and the SAS-compatibility caveats/disclaimer. Acceptance: README covers all shipped commands.
- [ ] **13.3 Compatibility matrix doc.** Publish the current compatibility percentages (from `ass test`) and the list of supported/unsupported constructs. Acceptance: doc generated from harness output.
- [ ] **13.4 Contributing guide.** How to add a corpus item, run the harness, and add a PROC/function. Acceptance: `CONTRIBUTING.md` exists.
- [ ] **13.5 Release checklist.** Versioning, build artifacts for cross-platform, license file. Acceptance: a tagged build runs the corpus green.

---

## Progress log

Append newest entries at the bottom. One entry per work session/step. Format:

```
### <date> — <step id(s)>
- What changed:
- Key files:
- Decisions/deviations:
- Next:
```

### 2026-06-16 — plan created
- What changed: Authored this development plan (`PLAN.md`).
- Key files: PLAN.md
- Decisions/deviations: Plan follows the compatibility levels and milestones in `ass-design.md`. No code written yet — repository still contains only `ass-design.md` and `CLAUDE.md`.
- Next: Phase 0.1 — initialize the Go module (confirm module path with user).

### 2026-06-16 — Phase 0 complete (0.1–0.4)
- What changed: Scaffolded the Go project end to end. Module initialized, all packages stubbed, CLI runs, committed to git.
- Key files: `go.mod` (module `github.com/solifugus/ass`, go 1.24), `.gitignore`, `README.md`, `doc.go` in all 11 packages (lexer, parser, ast, macro, runtime, table, proc, formats, sql, log, corpus — `vm` deferred per plan), `cmd/ass/main.go`.
- Decisions/deviations: Module path `github.com/solifugus/ass` (confirmed with user). Git enabled (confirmed). Toolchain: Go 1.24.4. CLI dispatches `--help`/`test`/`<file>`; file-run and test are placeholders pending the lexer (Phase 2) and harness (Phase 11). Committed as "Phase 0: project scaffolding" (1a2b686).
- Verified: `go build ./...`, `go vet ./...` clean; `go run ./cmd/ass --help`, `... ass-design.md`, `... test corpus/` all behave as intended.
- Next: Phase 1.1 — write `corpus/FEATURES.md` (canonical feature-tag list from design §9).

### 2026-06-16 — Phase 1 complete (1.1–1.6)
- What changed: Built out the research/sample-gathering phase. Defined the feature-tag vocabulary, the corpus item format, and authored 18 hand-written corpus items spanning Levels 1–4.
- Key files:
  - `corpus/FEATURES.md` — canonical feature-tag table (tag → level → meaning), synced with design §3/§9.
  - `corpus/README.md` — on-disk item layout, `meta.yaml` schema, and a worked example.
  - 18 corpus items, each `corpus/<id>/{input.sas,meta.yaml}`:
    - Level 1 (6): data_step_basic_001, data_step_assignment_001, data_step_subset_if_001, data_step_if_then_else_001, data_step_do_loop_001, data_step_n_001.
    - Level 2 (5): proc_print_var_001, proc_print_noobs_001, proc_sort_byvars_001, proc_sort_descending_001, proc_sort_nodupkey_001.
    - Level 3 SQL (4, priority 2): sql_select_001, sql_create_table_001, sql_join_001, sql_groupby_001.
    - Level 4 macro (3, priority 2): macro_let_001, macro_def_001, macro_control_001.
- Decisions/deviations: All items are hand-written, MIT-licensed (no external sources pulled in yet — keeps licensing clean). Every item is `output: unverified` for now; the expected *content* of each is described in its `meta.yaml notes:`. **Step 1.6 was revised**: expected_output.txt files are deferred to Phase 5.2 (PROC PRINT renderer defines exact spacing — writing them now would bake in arbitrary formatting). Added a reminder to 5.2 to backfill them and flip to `verified`.
- Verified: `go build ./...` still clean (no Go code changed). Corpus item dirs confirmed to each contain input.sas + meta.yaml. meta.yaml feature tags all exist in FEATURES.md (manual check).
- Next: Phase 2.1 — define token types in `lexer/token.go`.

### 2026-06-16 — Phase 2 complete (2.1–2.5)
- What changed: Implemented the lexer end to end with full test coverage, and wired it into the CLI.
- Key files:
  - `lexer/token.go` — `TokenType` (readable string constants) and `Token{Type,Literal,Line,Col}`; structural keywords (DATA/PROC/RUN/QUIT/DATALINES via `LookupIdent`, case-insensitive); operators incl. `**` POW, `||` CONCAT, mnemonic-symbol ops; `$` DOLLAR; `DATALINES_DATA` for raw blocks.
  - `lexer/lexer.go` — rune-based `Lexer` with line/col tracking; `NextToken`/`scan`; `skipTrivia` handles `/* */` block comments and `* ... ;` statement comments (statement-start tracked via `lastType`, seeded to SEMICOLON); `readDatalines` raw-data mode (verbatim lines until a `;`/`;;;;` terminator line, which is left for the normal scanner); string `''`/`""` escape handling; numbers with fraction/exponent.
  - `lexer/lexer_test.go` — table tests: basic stream, operators, numbers, strings (with escapes), case-insensitive keywords, positions, illegal rune, block/statement comments, `*`-is-multiply, `$` in input, full datalines flow.
  - `cmd/ass/main.go` — `runFile` now prints the token stream (`line:col TYPE "literal"`); execution-pending note goes to stderr.
- Decisions/deviations: Mnemonic word operators (`eq`, `and`, `or`, ...) are intentionally left as IDENT for the parser to classify (Phase 3.4) rather than lexed as operators — keeps the lexer context-free. **Discovered caveat:** `data=` in `proc print data=ds` lexes as the `DATA` keyword token (SAS keywords are contextual); recorded a note on step 3.6 so the PROC option parser accepts keyword tokens as option names.
- Verified: `go test ./...` green (lexer suite passes; other packages have no tests yet). `go build`/`go vet` clean. Manual: `go run ./cmd/ass corpus/data_step_basic_001/input.sas` produces a correct token dump including the `DATALINES_DATA "John 25\nMary 30"` block.
- Next: Phase 3.1 — define AST node interfaces and the program/step containers in `ast/`.

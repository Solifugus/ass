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

- [x] **3.1 AST node definitions.** In `ast/`, define interfaces (`Node`, `Statement`, `Expression`) and the program/step containers: `Program` (list of steps), `DataStep`, `ProcStep` (generic with proc name + raw options/statements for now). Acceptance: compiles.
- [x] **3.2 Statement & expression nodes.** Add nodes for: assignment, `set`, `input` (with var list + `$` flags), `datalines` (raw block), `if/then/else`, `do/end`, `output`, `keep/drop`, plus expression nodes (literal, identifier, binary op, unary op, function call). Acceptance: compiles; nodes have `String()` for debugging.
- [x] **3.3 Program/step parser.** In `parser/parser.go`, implement top-level parsing that splits the token stream into steps by `data`/`proc` ... `run;`/`quit;` and dispatches to step parsers. Add `parser/parser_test.go`. Acceptance: parses a multi-step file into a `Program` with the right step count/types.
- [x] **3.4 Expression parser (Pratt).** Implement precedence-climbing/Pratt parsing for SAS expressions: arithmetic (`+ - * / **`), comparison (`= ^= < <= > >=` and word forms `eq ne lt`...), logical (`and or not`), function calls, parentheses. Add tests. Acceptance: expression tests pass including precedence.
- [x] **3.5 DATA step statement parser.** Parse the bodies of DATA steps into the statement nodes from 3.2. Add tests against Level-1 corpus items. Acceptance: Level-1 items parse to expected AST shape.
- [x] **3.6 PROC step option parser.** Parse common PROC option syntax: `proc <name> data=<ds> (options);` and statement lines like `by`, `var`, `where`. Keep proc-specific semantics out (just structure). **Caveat from Phase 2:** the lexer emits `data` as the `DATA` keyword token even in `proc print data=people` (SAS keywords are contextual). The option parser must accept the `DATA` token (and likely `SET`, etc.) where an option name is expected, rather than only `IDENT`. Acceptance: PROC PRINT/SORT items parse.
- [x] **3.7 Wire parser into CLI.** Add `ass parse <file.sas>` (or a flag) that prints the AST. Acceptance: prints AST for corpus items.

## Phase 4 — DATA step runtime (the core)

- [x] **4.1 Dataset model.** In `table/`, implement `Dataset` (library name, name, ordered columns with name/type/label/format/informat, rows as `[]Row`), `Column`, and a `Value` type supporting numeric, character, and **missing** (`.` numeric, `''` char). Add tests for construction and missing-value semantics. Acceptance: `go test ./table/` passes.
- [x] **4.2 In-memory library/catalog.** Implement a `Library` map (e.g. `work`) holding datasets by name, with get/put. This is how steps pass data. Acceptance: tests for put/get and overwrite.
- [x] **4.3 PDV + expression evaluator.** In `runtime/`, implement the Program Data Vector (variable name → current Value) and an expression evaluator over the PDV (numbers, strings, missing propagation, arithmetic, comparison returning 1/0, logical ops, a few core functions like `sum`, `upcase`). Add tests. Acceptance: evaluator tests pass including missing-value propagation rules.
- [x] **4.4 Implicit loop & assignment/output.** Implement the DATA step driver: initialize PDV, run the implicit row loop, execute assignment statements, handle automatic `_N_` (iteration counter) and `_ERROR_`. For a step with no input, run once. Implement explicit/implicit `output`. Acceptance: a `data` step with assignments produces the expected dataset.
- [x] **4.5 `input` + `datalines`.** Implement reading the raw datalines region per the `input` spec (list input: space-delimited, `$` = char). Populate the PDV and output one row per data line. Acceptance: the canonical `data people; input name $ age; datalines; ... run;` produces a 2-row dataset.
- [x] **4.6 `set` (read existing dataset).** Implement `set <ds>`: the implicit loop iterates rows of the input dataset into the PDV. Acceptance: `data b; set a; run;` copies dataset a to b.
- [x] **4.7 `if/then/else` + subsetting if.** Implement conditional execution and the subsetting-`if` (a bare `if cond;` that drops the row when false). Acceptance: `data adults; set people; if age>=18; run;` filters correctly.
- [x] **4.8 `do/end` loops & `keep`/`drop`.** Implement iterative `do`/`do while`/`do until` and column selection via `keep`/`drop` statements (and dataset options if feasible). Acceptance: tests for a do loop and keep/drop pass.
- [x] **4.9 SAS-style log output.** In `log/`, implement a logger that writes SAS-like NOTEs (e.g. "NOTE: The data set WORK.PEOPLE has N observations and M variables."). Wire DATA step to emit these. Acceptance: log lines match the expected format for Level-1 items.

## Phase 5 — PROC PRINT

- [x] **5.1 PROC dispatch.** In `proc/`, define a `Proc` interface (run against a Library + step AST + logger) and a registry mapping proc name → implementation. Wire the runtime to dispatch PROC steps. Acceptance: unknown procs produce a clean "not supported" log note, not a crash.
- [x] **5.2 PROC PRINT core.** Implement `proc print data=<ds>;` rendering a SAS-like listing (Obs column + variables, right/left alignment by type). Support `var` to select/order columns. Add tests comparing rendered text to expected. Acceptance: PROC PRINT renders match hand-written expected listings (unit tests in `proc/`). **DECISION (2026-06-16):** the corpus `expected_output.txt` → `verified` backfill (originally folded in here / step 1.6) is **deferred** and NOT done from the ASS engine's own output. Per `corpus/README.md`, `verified` means *hand-derived from real SAS*, which this environment can't produce; capturing our own listing and calling it verified would mislabel it. Backfill is blocked on (a) access to real SAS output, or (b) a Phase 11 decision on whether ASS's clean-room listing format is itself the comparison target (and an added `regression`/`baseline` output state if so). Items stay `output: unverified` until then. New tracking step 11.x to be added in Phase 11.
- [x] **5.3 PROC PRINT options.** Add `noobs`, `label` (use column labels as headers). Acceptance: tests for noobs/label pass.

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

### 2026-06-16 — Phase 3 partial (3.1–3.3)
- What changed: Defined the AST node set and built the top-level parser that splits a program into steps. Step headers and PROC options are parsed; step bodies are captured (DATALINES structured; other statements as RawStatement, to be refined in 3.5).
- Key files:
  - `ast/ast.go` — `Node`/`Statement`/`Expression`/`Step` interfaces; `Program`, `DataStep` (Datasets + Body), `ProcStep` (Name/Data/Options/Body), `ProcOption`. All have `String()`.
  - `ast/expressions.go` — NumberLiteral, StringLiteral, MissingLiteral, Identifier, PrefixExpression, InfixExpression, CallExpression.
  - `ast/statements.go` — Assignment, Set, Input (+InputVar.Char), Datalines, If (Consequence/Alternative), SubsettingIf, Do (DoKind: simple/iterative/while/until), Output, Keep, Drop, By (+Descending), Var, and RawStatement (placeholder for not-yet-structured statements).
  - `parser/parser.go` — `Parser` with cur/peek lookahead; `ParseProgram` dispatches DATA/PROC; `parseDataStep`, `parseProcStep` (options incl. `data=`, accepts keyword tokens as option names per the Phase-2 caveat), `parseStepBody` (stops at run/quit), `parseStatement` (DATALINES structured, else raw), `parseDatalines`, `parseRawStatement`. Local `itoa` to avoid a one-call strconv import.
  - `parser/parser_test.go` — step counts/types, datalines capture, proc options+flags, no-errors-on-clean-input.
- Decisions/deviations: PROC option parsing (part of step 3.6) was done here since it's structural; 3.6 remains for body statements like `by`/`var`/`where` and any option-name edge cases. RawStatement lets the parser keep moving over assignment/set/input/if/do until 3.5 gives them real nodes — nothing is dropped.
- Verified: `go test ./...` green (lexer + parser); `go vet ./...` clean.
- Next: Phase 3.4 — Pratt expression parser (arithmetic/comparison/logical/calls, incl. mnemonic word operators eq/ne/and/or/not).

### 2026-06-16 — Phase 3 complete (3.4–3.7)
- What changed: Finished the parser — Pratt expression parser, real DATA-step and PROC statement parsing (replacing RawStatement placeholders), and a `parse` CLI command. Verified by parsing every non-macro corpus item cleanly.
- Key files:
  - `parser/expression.go` — precedence-climbing parser. Precedence (low→high): or, and, compare, ||, +/-, * /, prefix, ** (right-assoc). Mnemonic word operators (and/or/eq/ne/lt/le/gt/ge) normalized to symbolic form; `not`/`^`/`~` prefix binds looser than comparison (matches SAS for `not a = b`). Function calls and parenthesized grouping.
  - `parser/statements.go` — `parseDataStatement` (set, input+$, if/then/else, subsetting if, do/while/until/iterative + end, output, keep, drop, by, assignment) and `parseProcStatement` (by, var; else raw). Helpers: identIs/peekIdentIs, parseDatasetNames (dotted lib.name).
  - `parser/parser.go` — `parseStepBody` now takes a statement-parser func; DATA uses parseDataStatement, PROC uses parseProcStatement.
  - `parser/expression_test.go`, `parser/statements_test.go` — precedence/associativity, mnemonics, calls, literals; assignment/input, set/subsetting-if, if/then/else, do-loop+output, proc by/var.
  - `cmd/ass/main.go` — `ass parse <file.sas>` prints the AST and reports parse errors (non-zero exit on errors).
  - `ast/ast.go`,`expressions.go`,`statements.go` — added nil-safe `str()` helper; all child-rendering `String()` methods use it.
- Decisions/deviations: Found and fixed a robustness bug — `String()` panicked on partial trees from error-parses (nil children); now renders `<?>`. Macro corpus items (`&var`, `%let`, `%macro`) intentionally still produce parse errors: they require the Phase 9 macro preprocessor that runs *before* the parser. All Level-1/2 and SQL items parse with zero errors. PROC option parsing was completed earlier in 3.3.
- Verified: `go test ./...` green; `go vet` clean; `ass parse` over all 15 non-macro corpus items yields zero errors; macro items error gracefully (no panic).
- Next: Phase 4.1 — dataset model in `table/` (Dataset, Column, Value with missing-value semantics).

### 2026-06-16 — Phase 4 partial (4.1–4.2)
- What changed: Built the `table` package — the dataset/value model and the in-memory library — the data substrate the runtime will fill.
- Key files:
  - `table/value.go` — `Kind` (Numeric/Character); `Value{Kind,Num,Str,missing}` with constructors `Num`/`Char`/`MissingNum`/`MissingChar`; `IsMissing()` (numeric uses flag, character missing == ""); `Display()` ("." for num missing, compact decimal otherwise).
  - `table/dataset.go` — `Column` (Name/Kind/Label/Format/Informat/Length); `Row = map[string]Value` (lowercased keys); `Dataset{Lib,Name,Columns,Rows}` with `NewDataset` (defaults lib WORK), `AddColumn` (dedup case-insensitive), `HasColumn`, `ColumnNames`, `AppendRow`, `NObs`, `Get` (case-insensitive, returns typed missing for absent columns).
  - `table/library.go` — `Library` keyed by uppercased name; `Put`/`Get`/`Has`/`Names`; `Get` accepts qualified `lib.name` (lib component ignored for now — single in-memory library).
  - `table/table_test.go` — missing-value semantics, Display formatting, column dedup + typed-missing Get, library put/get/overwrite/qualified-name.
- Decisions/deviations: Row is a map keyed by lowercased name (convenient for the PDV in 4.3); column order/metadata lives in Columns. Blank-string-as-missing (SAS treats " " as missing) is NOT yet normalized — only "" is missing; noted via a skipped test, to revisit in the formats phase. Qualified `lib.name` resolves on the dataset component only (everything is in one library until persistent storage lands).
- Verified: `go test ./table/` green; `go vet ./...` clean.
- Next: Phase 4.3 — PDV + expression evaluator in `runtime/` (evaluate ast.Expression over a PDV with SAS missing-value propagation).

### 2026-06-16 — Phase 4.3 (PDV + expression evaluator)
- What changed: Built the `runtime` package core — the Program Data Vector and a SAS-semantics expression evaluator over it. This is the first executable piece of the DATA step engine; the implicit loop (4.4) will drive it.
- Key files:
  - `runtime/pdv.go` — `PDV` maps case-insensitive var names → `table.Value`, tracks each variable's type (fixed at first appearance) and first-seen display order. `Set`/`Get`/`Has`/`Declare`/`Kind`/`Names`/`ResetVars`. Undeclared var reads numeric missing; declared-but-unset reads typed missing. `ResetVars` clears values but preserves declarations/order (for the per-iteration PDV clear).
  - `runtime/eval.go` — `Eval(ast.Expression, *PDV) (table.Value, error)`. Arithmetic propagates missing (incl. div-by-zero → missing); comparisons return 1/0 with numeric missing ordered below all numbers and char compared lexically; `and`/`or` short-circuit on truthiness (missing & 0 are false), return 1/0; `not`; `||` concat; `**`. `truthy` = non-missing non-zero number.
  - `runtime/functions.go` — `evalCall`: aggregates that ignore missing (`sum`/`mean`/`min`/`max`/`n`/`nmiss`, all-missing → missing); scalar math (`abs`/`int`/`ceil`/`floor`/`sqrt`/`exp`/`log`/`round`) propagating missing; string fns (`upcase`/`lowcase`/`trim`/`strip`/`left`/`length`/`substr`/`cats`).
  - `runtime/eval_test.go`, `runtime/pdv_test.go` — arithmetic/precedence, missing propagation, comparison incl. missing-sorts-low, logical, concat, function library, PDV type-fixing/order/reset.
  - `parser/parser.go` — added exported `ParseExpressionString(src)` helper (parses one expression in isolation; nil on error) used by runtime tests.
- Decisions/deviations: Evaluator returns a Go `error` for genuinely unsupported constructs (unknown function/operator); SAS-style "set _ERROR_ and note in log" handling is deferred to the loop driver (4.4) / log phase (4.9). Char-in-logical-context is truthy iff non-empty (SAS normally requires numeric); revisit if a corpus item needs it.
- Verified: `go test ./runtime/ ./parser/` green; `go build ./...` and `go vet ./...` clean.
- Next: Phase 4.4 — implicit loop & assignment/output. Build the DATA step driver in `runtime/`: init PDV, run the implicit row loop, execute `AssignmentStatement`s via `Eval`, maintain `_N_`/`_ERROR_`, handle `output` (explicit + implicit-at-iteration-end), and write rows to a `table.Dataset` in a `table.Library`. For a step with no input, run one iteration.

### 2026-06-16 — Phase 4.4 (implicit loop & assignment/output)
- What changed: Built the DATA step driver — the first end-to-end execution path (source → dataset). Handles input-less steps: one implicit-loop iteration, assignment via `Eval`, and explicit/implicit OUTPUT.
- Key files:
  - `runtime/datastep.go` — `RunDataStep(ds *ast.DataStep, lib)` creates output dataset(s) (unnamed step → `DATA1`), seeds `_n_`=1/`_error_`=0, runs the body once, and does implicit output at iteration end unless the step contains an explicit OUTPUT (`containsOutput` scans nested if/do bodies too). `dataStep` struct holds per-run state; `execStatement` dispatches `AssignmentStatement`/`OutputStatement` (other kinds are no-ops until later phases). `writeRow` appends the PDV (excluding automatic `_n_`/`_error_`) in declaration order, declaring columns on the dataset as it goes. `flow` enum (`flowNormal`/`flowDelete`) is in place for the subsetting-if / delete path coming in 4.7.
  - `runtime/datastep_test.go` — single-row assignments + types, column order, automatic vars excluded but `_n_` readable, explicit OUTPUT suppresses implicit (and writes per-output PDV snapshots), implicit-output-once, unnamed-step default name.
- Decisions/deviations: No input source yet ⇒ exactly one iteration (the SET/INPUT loop arrives in 4.5–4.6). PDV is **not** reset between explicit outputs within an iteration (correct SAS behavior). Unsupported statement kinds are silently skipped so partially-implemented steps still run; this becomes stricter as kinds are added.
- Verified: `go test ./runtime/` green; `go build ./...` and `go vet ./...` clean.
- Next: Phase 4.5 — input + datalines. Read inline data: drive the implicit loop over `DatalinesStatement` lines, parse each line per the `InputStatement` var list (list input: whitespace-delimited, `$` ⇒ character), set PDV vars, output a row per line. Acceptance: a `data; input; datalines;` step produces one row per data line with correct types.

### 2026-06-16 — Phase 4.5 (input + datalines)
- What changed: The DATA step driver now reads inline data. INPUT + DATALINES drives the implicit loop one iteration per record.
- Key files (`runtime/datastep.go`):
  - `RunDataStep` now branches: if the body has an INPUT statement and datalines records, it loops `while recPtr < len(records)`, else runs one iteration. Extracted `runIteration` (resets PDV, bumps `_n_`, sets `_error_`=0, executes body, implicit-outputs unless explicit/deleted). `dataStep` gained `records`/`recPtr`.
  - `execStatement`: INPUT reads `records[recPtr]` via `applyInput` then advances (EOF ⇒ `flowDelete`, no output); DATALINES is a no-op (data collected up front).
  - `applyInput` — list input: `strings.Fields` split, positional match to the var list, `$` ⇒ `Char`, else `parseNum`; short records / "." / unparseable ⇒ typed missing. `parseNum`, `collectDatalines`, `hasInputStatement` helpers added.
  - `runtime/datastep_test.go` — 3-row name/age dataset (column order + numeric typing), computed column over input (`y = x*2`), missing trailing field ⇒ missing.
- Decisions/deviations: List input only (no column/formatted input, no `@`/`@@` line-hold) — sufficient for the corpus; richer input is a later DATA-step item. Each iteration fully resets the PDV (retain/sum semantics arrive in Phase 10). A safety bump guarantees loop progress if an iteration executes no INPUT.
- Verified: `go test ./runtime/` green; `go build ./...` and `go vet ./...` clean.
- Next: Phase 4.6 — `set`. Read rows from an existing library dataset to drive the implicit loop (one iteration per input row, PDV seeded from the row's columns; multiple datasets concatenate). Acceptance: `data b; set a; ... run;` copies/transforms `a` into `b`.

### 2026-06-16 — Phase 4.6 (set)
- What changed: The DATA step driver can now read an existing dataset. SET drives the implicit loop one iteration per input row; multiple datasets concatenate.
- Key files (`runtime/datastep.go`):
  - `RunDataStep` input-mode selection is now SET → INPUT/DATALINES → none. SET mode loops `while setPtr < len(setRows)`. `dataStep` gained `setRows []sourceRow` / `setPtr`; `sourceRow{row, ds}` pairs each row with its source dataset for column types.
  - `execStatement`: SET reads `setRows[setPtr]` via `applySet` then advances (exhausted ⇒ `flowDelete`). `applySet` seeds the PDV from the source dataset's columns in order (so SET vars lead the output layout), typed via `ds.Get`.
  - `collectSetRows` resolves the SET datasets from the library up front and concatenates their rows in statement order (unknown datasets skipped); `hasSetStatement` selects the mode.
  - `runtime/datastep_test.go` — `runProgram` helper (runs every DATA step in a program against one library); SET copy + computed column with column ordering (set vars first), and `set a c` concatenation (2 + 1 = 3 rows).
- Decisions/deviations: SET rows are snapshotted at step start (source row maps shared read-only — fine, output is a distinct dataset). Each iteration still fully resets the PDV then SET repopulates; this matches copy/transform semantics. `BY`/`first.`/`last.`, `merge`, `in=`, and `retain` are Phase 10 (L5). If both SET and INPUT appear, SET wins for now.
- Verified: `go test ./runtime/` green; `go build ./...` and `go vet ./...` clean.
- Next: Phase 4.7 — if/then/else + subsetting if. Execute `IfStatement` (eval condition truthiness → run consequence/alternative, which are themselves statements) and `SubsettingIf` (`if cond;` ⇒ drop the row when false, i.e. return `flowDelete`). Acceptance: subset corpus items produce only matching rows.

### 2026-06-16 — Phase 4.7 (if/then/else + subsetting if)
- What changed: Conditional execution in the DATA step. `IfStatement` evaluates its condition's truthiness and runs the consequence or (optional) alternative statement; `SubsettingIf` (`if cond;`) drops the current row when false.
- Key files (`runtime/datastep.go`): `execStatement` gained `*ast.IfStatement` (delegates to `execStatement` on the chosen branch, so flow signals like a nested OUTPUT or subsetting-if propagate) and `*ast.SubsettingIf` (false ⇒ `flowDelete`). Reuses `Eval` + `truthy` from the evaluator.
- Key files (`runtime/datastep_test.go`): subsetting-if filters by age; if/then/else assigns P/F grades; `if x>5 then output;` writes only matching rows (the THEN-output makes the step explicit-output via `containsOutput`, suppressing implicit output).
- Decisions/deviations: A THEN/ELSE branch is a single statement (the parser already wraps multi-statement branches in DO blocks, handled in 4.8). No `delete`/`return`/`abort` statements yet (no AST nodes); subsetting-if covers the corpus's filtering needs.
- Verified: `go test ./runtime/` green; `go build ./...` and `go vet ./...` clean.
- Next: Phase 4.8 — do/end + keep/drop. Execute `DoStatement` (simple/iterative/while/until — run the body, iterative drives a loop var via `From`/`To`/`By`, while/until test `Cond`) and apply `KeepStatement`/`DropStatement` to filter the output variable set. Acceptance: a `do i = 1 to n` loop and a kept/dropped variable list behave correctly.

### 2026-06-16 — Phase 4.8 (do/end loops + keep/drop)
- What changed: DO...END blocks and output-variable selection. All four DO forms execute; KEEP/DROP filter the columns written to output.
- Key files (`runtime/datastep.go`):
  - `execStatement` gained `*ast.DoStatement` → `execDo`. `execDo` handles `DoSimple` (run body once), `DoWhile` (test-before), `DoUntil` (test-after, ≥1 run); `execDoIterative` runs `do var = from to to [by by]`, leaves the loop var one step past the bound (SAS `do i=1 to 3` ⇒ i=4), and skips on missing/zero-step bounds to avoid non-termination. A `flowDelete` from any DO body propagates out of the loop and the iteration.
  - `dataStep` gained `keep`/`drop` (lowercased sets); `collectKeepDrop` scans the body (keep nil ⇒ keep-all, accumulates across statements); `writeRow` skips automatic, dropped, and non-kept vars.
  - `runtime/datastep_test.go` — iterative loop with output + terminal loop-var value, `by` step count, do-while (3 rows), do-until (runs once when already past bound), drop removes a column, keep restricts to a column list (order preserved).
- Decisions/deviations: KEEP/DROP are treated as step-global (scanned up front, top-level), matching how SAS compiles them; dataset-option forms (`data out(keep=...)`) are deferred. No `leave`/`continue`/`do over`/array-do yet (no AST nodes); arrays are Phase 10 (L5).
- Verified: `go test ./runtime/` green; `go build ./...` and `go vet ./...` clean.
- Next: Phase 4.9 — SAS-style log output. In `log/`, add a logger emitting NOTEs (e.g. "NOTE: The data set WORK.OUT has N observations and M variables."); wire `RunDataStep` to emit one per output dataset. Acceptance: log lines match the expected format for L1 items.

### 2026-06-16 — Phase 4.9 (SAS-style log) — PHASE 4 COMPLETE
- What changed: Added the `log` package and wired the DATA step to emit the standard post-step NOTE. This closes Phase 4 — the DATA step runtime is end-to-end for L1.
- Key files:
  - `log/log.go` — `Logger` wrapping an `io.Writer`; `Note`/`Warning`/`Error` (printf-style, SAS prefixes) and `DatasetNote(lib, name, nobs, nvars)` → "NOTE: The data set WORK.NAME has N observations and M variables.". A nil `*Logger` is safe (discards), so callers needn't guard.
  - `runtime/datastep.go` — `RunDataStep` now takes a `*log.Logger` (nil-safe) and emits a `DatasetNote` per output dataset after `Put`.
  - `log/log_test.go` — level prefixes, dataset note format, nil-logger safety. `runtime/datastep_test.go` call sites pass `nil`.
- Decisions/deviations: Signature change (added logger param) over a stateful runner struct — simplest for now; Phase 5's CLI will pass a real logger writing to stderr. Variable-count uses `len(out.Columns)` (post keep/drop, since columns are only added on output).
- Verified: `go test ./...` green across lexer/parser/table/runtime/log; `go build ./...` and `go vet ./...` clean.
- Next: Phase 5 — PROC PRINT + the `ass run` CLI path. First wire `cmd/ass` to execute a program (lex → parse → for each step: DATA via `RunDataStep`, PROC via a dispatcher) with a real logger to stderr; then implement PROC PRINT (`proc/`) rendering a dataset as the SAS-style listing (Obs column unless `noobs`, `var` selection), and start backfilling corpus `expected_output.txt`. See step 5.1/5.2 for specifics.

### 2026-06-16 — Phase 5.1 (PROC dispatch + `ass run` CLI path)
- What changed: The execution pipeline is now wired end to end. A program runner dispatches DATA and PROC steps, and `ass <file.sas>` executes a program (not just dumps tokens).
- Key files:
  - `proc/proc.go` — `Proc` interface (`Run(lib, *ast.ProcStep, logger) error`); a name→impl `registry` with `Register` (panics on dup) / `Lookup`; package-level `Run` that dispatches, logging "NOTE: PROC X is not supported and was skipped." for unregistered procs (no crash).
  - `runtime/program.go` — `RunProgram(prog, lib, logger)`: iterates steps, DATA → `RunDataStep`, PROC → `proc.Run`; stops at first error. (runtime→proc import; no cycle.)
  - `cmd/ass/main.go` — `ass <file>` now runs the program (parse-errors abort before exec; log → stderr, PROC output → stdout); token dump moved to `ass tokens <file>`; usage updated.
  - `runtime/program_test.go` — DATA builds PEOPLE then unknown PROC PRINT logs not-supported.
- Verified: `go test ./runtime/ ./proc/` green; `go build`/`go vet` clean. Ran all L1 corpus items via `go run ./cmd/ass`: correct obs/var counts (sales 3×4, people→adults 4→2, squares 5, graded 3×3), PROC PRINT logs not-supported (lands in 5.2).
- Next: Phase 5.2 — PROC PRINT core. Implement `proc print data=<ds>;` in `proc/` (register as "print"): SAS-style listing with an Obs column and the variables (numeric right-aligned, char left-aligned), honoring `var` for column selection/order, writing to stdout. Add render tests comparing to expected text. Then backfill corpus `expected_output.txt` for L1/L2 items and flip `output: unverified` → `verified`.

### 2026-06-16 — Phase 5.2 (PROC PRINT core)
- What changed: Implemented PROC PRINT — the first PROC and the first thing to produce user-facing output. The full pipeline now reads SAS and prints a listing.
- Key files:
  - `proc/print.go` — `printProc` registered as "print" via `init()`. `Run` resolves `data=`, renders to stdout, logs "NOTE: There were N observations read from the data set LIB.NAME.". `renderListing` is a pure function (testable): Obs column (1-based, suppressed by `noobs`) + selected columns; numeric right-aligned, character left-aligned; headers align with data; two-space gutter; blank line under the header; trailing blanks trimmed per line. `selectColumns` honors `var` (existing columns, given order) else all columns. **This is the locked "ASS listing format"** (our own clean-room format, not byte-identical to SAS).
  - `proc/print_test.go` — exact-string tests: default listing, `noobs`, var selection (single + reorder), numeric missing renders ".".
  - `runtime/program_test.go` — updated the unknown-PROC test to use PROC FREQ (PRINT is now registered).
- Decisions/deviations: See the 5.2 plan line — corpus `expected_output.txt`/`verified` backfill is deferred (can't hand-derive from real SAS here; won't self-verify from our own engine). `data=` only (no `_LAST_` default yet) — missing dataset logs an ERROR note and skips. Numeric display uses `Value.Display()` (compact `%g`); BEST.-style width formatting is Phase 6.3.
- Verified: `go test ./...` green (lexer/parser/table/runtime/log/proc); `go build`/`go vet` clean. End-to-end via `go run ./cmd/ass`: basic_001 prints the John/Mary listing; proc_print_var_001 prints item/total (qty, price correctly omitted).
- Next: Phase 5.3 — PROC PRINT options: `label` (use column `Label` as header when set, falling back to name) and confirm `noobs` (already implemented in 5.2). Add tests. Then Phase 6 (functions/where/coercion polish) or Phase 7 (PROC SORT).

### 2026-06-16 — Phase 5.3 (PROC PRINT options) — PHASE 5 COMPLETE
- What changed: PROC PRINT now honors `label` (use a column's Label as its header when set); `noobs` was already implemented in 5.2. Closes Phase 5.
- Key files:
  - `proc/print.go` — `printOptions.label`; `parsePrintOptions` recognizes `label`/`noobs` (case-insensitive); `listingColumn` gained a `header` field; `renderListing` uses the label as header (and for width) when `label` is set and the column has one, else the variable name.
  - `proc/print_test.go` — label header widens/right-aligns correctly; no-label falls back to the name.
- Decisions/deviations: `label` is the proc-option form (`proc print data=x label;`); per-variable `label` statements inside the step are not parsed yet (no dedicated AST node) — labels come from column metadata set elsewhere (e.g. future LABEL statement / dataset attrs). Header alignment follows the column's data alignment.
- Verified: `go test ./...` green; `go build`/`go vet` clean.
- Next: Phase 6 — Expressions/functions/filtering polish. 6.1 expand the DATA-step function library (`scan`, `index`, plus rounding out the set already present); 6.2 `where` (statement + dataset option) vs subsetting `if`; 6.3 type coercion + BEST.-style numeric formatting. Alternatively jump to Phase 7 (PROC SORT) to light up the L2 sort corpus. Recommend 7 next for breadth of runnable corpus, then circle back to 6.

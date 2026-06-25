# Testing plan

ASS is feature-complete for its intended scope (DATA step, core PROCs, macro
basics, LIBNAME engines, the Jupyter kernel). The next phase is **confidence**:
proving the implemented surface is correct, broad, and robust enough to trust on
real industry workloads before the project shifts to adoption/business work.

This document is the working plan for that phase. It defines four tracks, how
each is measured, and the concrete backlog. It is a living document — append to
the backlog as issues are found, and check items off as they land.

How correctness is measured here has not changed: **values, not byte-identical
presentation** (see [`COMPATIBILITY.md`](COMPATIBILITY.md)). Every test asserts
dataset columns/values, SQL result sets, or computed statistics — never the
exact spacing of a PROC listing.

## Where we are today

- `ass test corpus/` reports **62/62** items parsed / executed / passed (100%).
- **34/34** items that declare `expected.datasets` value-match (100%).
- Go unit tests cover the lexer, parser, macro, runtime, formats, proc, sql,
  table, session, kernel, and log packages.

The gap is **breadth and adversarial depth**: the corpus exercises features in
isolation on small, well-formed inputs. It does not yet stress missing-value
edge cases, type-coercion boundaries, malformed input, multi-step industry
pipelines, or comparison against externally-published canonical results.

## Track 1 — Grow the value-verified corpus

**Goal:** every implemented statement, function, format, informat, dataset
option, and PROC has at least one corpus item with hand-derived
`expected.datasets`, pushing the value-verified count well past 34.

**Method**

- Inventory the implemented surface from [`reference.md`](reference.md) and map
  each item to the corpus features that cover it. Tag gaps.
- For each gap, add a corpus item (`corpus/*.sas` + YAML metadata) whose
  `expected.datasets` are hand-derived from SAS semantics — no SAS license
  needed, because the data semantics are deterministic.
- Prioritize the surfaces the industry cookbooks lean on hardest: BY-group
  `first.`/`last.`, `merge`/`in=`, `retain`/sum statement, date functions
  (`intck`/`intnx`/`mdy`), user formats/informats, and PROC
  FREQ/MEANS/SQL/REG/PROOF.

**Done when:** a coverage report (extend `ass test --json`) shows every
reference.md surface item has ≥1 value-verified corpus item, and the
value-verified total tracks the parsed total.

## Track 2 — Industry end-to-end programs

**Goal:** turn each industry cookbook into a runnable, asserted integration test
so the multi-step pipelines real analysts write keep working.

**Method**

- The four cookbooks ([pharma](cookbook-pharma.md), [banking](cookbook-banking.md),
  [insurance](cookbook-insurance.md), [government](cookbook-government.md)) were
  authored by running every program through the engine. Promote the substantive
  multi-step recipes into corpus items with `expected.datasets`.
- Add at least one *full-pipeline* item per industry: read (CSV/datalines) →
  clean/derive → merge → aggregate → PROC PROOF gate → report. This exercises
  step-to-step dataset hand-off, not just one feature.
- Wire a CI check that re-runs each cookbook's programs and fails if any emits an
  unexpected `ERROR` (the intentional PROC PROOF fail-gate recipes are allowed to
  exit non-zero and are marked as such).

**Done when:** each industry has a value-verified end-to-end corpus item, and a
doc-test pass guarantees the cookbooks never drift from engine behavior.

## Track 3 — Edge-case & robustness

**Goal:** the engine behaves correctly (or fails cleanly) at the boundaries,
not just on tidy inputs.

**Areas**

- **Missing-value semantics:** propagation through arithmetic, comparison sort
  order (`.` sorts low), aggregate functions ignoring missing, `sum` statement
  treating missing as 0, `in=`/`first.`/`last.` with missing keys.
- **Type coercion:** numeric↔character in mixed expressions, concatenation of
  numerics, comparison across types, informat over-/under-width.
- **Format/informat boundaries:** width overflow (`best.` fallback to
  scientific), zero/negative/huge values, date edge cases (29FEB, year
  boundaries, `mdy` with invalid dates → missing).
- **Malformed input:** short/long datalines records, bad delimiters, `dsd`
  quoting edge cases, non-numeric in a numeric field, truncated files — assert a
  clean error or documented recovery, never a panic.
- **Scale:** a large (≥1M-row) DATA step and PROC SORT/MEANS/SQL run to confirm
  no quadratic blowups (cross-reference [`perf.md`](perf.md)).
- **Property/fuzz-style:** generate random small programs over the supported
  grammar and assert parse-stability (no panics) and round-trip invariants
  (e.g. `sort` then `sort` is idempotent; CSV write→read preserves values).

**Done when:** a robustness test package covers each area above, and a fuzz
target over the lexer/parser runs clean in CI.

## Track 4 — Differential vs published reference results

**Goal:** where a canonical SAS result is *publicly documented*, assert ASS
matches it — without a SAS license and without copying proprietary material.

**Method (clean-room)**

- Source expected results only from **public, citable** material: textbook
  worked examples, published statistical datasets with known answers, public
  documentation examples that include their output, and first-principles
  hand-derivation. Record the citation in the corpus item metadata.
- Build a small set of canonical-answer items: e.g. a known OLS regression
  (slope/intercept/R² hand-derivable), a chi-square on a textbook contingency
  table, summary stats on a classic public dataset.
- These become the highest-trust corpus items because the expected values are
  externally anchored, not self-derived.

**Boundary:** never reproduce proprietary SAS documentation, output listings, or
non-public behavior. Differential testing here means "matches the math the world
agrees on," not "matches SAS byte-for-byte" (an explicit non-goal).

**Done when:** a `differential` feature tag groups the externally-anchored items
and they pass.

## Known issues found while authoring the cookbooks

Surfaced by running real industry programs through the engine. These are the
first concrete backlog items for Track 1/Track 3.

- [x] **`rename` as a DATA-step statement was silently ignored.** Fixed — `data
  b; set a; rename x=xx; run;` now renames the output variable (the original name
  is used within the step; FORMAT/LABEL/KEEP by the original name still apply).
  Regression: corpus `data_step_rename_001` (value-verified) plus parser/runtime
  unit tests.
- [x] **Two-way PROC FREQ ignored `nofreq`/`nopercent`/`norow`/`nocol`.** Fixed —
  the cross-tab now drops the matching cell statistic (frequency / cell % / row %
  / col %); suppressing all four falls back to frequency. Cross-tab output is
  listing text (not a dataset), so the guard is a `proc` unit test rather than
  `expected.datasets`.
- [x] **PROC MEANS had no `maxdec=` and no `sum` keyword.** Fixed — the statistic
  keywords (`n`/`mean`/`std`/`stddev`/`min`/`max`/`sum`) now select which stats
  appear and in what order (default `N Mean StdDev Min Max`), and `maxdec=k` fixes
  the displayed decimals (N stays integer). Guard: `proc` unit tests.
- [ ] **No PROC TRANSPOSE.** Wide↔long reshaping is done with DATA-step arrays
  (documented in the general cookbook). TRANSPOSE is a frequent SAS idiom worth
  considering for a future phase. (Still open — larger feature, deferred.)

Verified **not** broken (initially suspected, then disproven):

- Colon list informats for dates (`input start : date9.;`) parse correctly —
  `15JAN2020` → `21929`. The value displays raw only because no format is
  attached, which is expected.
- PROC SQL runs in the pure-Go (`CGO_ENABLED=0`) build; the old "requires a CGo
  build" note in reference.md was stale and has been corrected.

## Sequencing

1. Land the **known-issue** fixes above (small, high-value, each with a corpus
   item that would have caught it).
2. Track 2 (industry end-to-end items) — converts work already done into
   regression protection.
3. Track 1 (systematic surface coverage) — the bulk, done feature-by-feature.
4. Track 3 (robustness/fuzz) and Track 4 (differential) in parallel as depth
   passes.

Each landed item follows the standard gate: gofmt/build/vet/test (both CGO
modes), corpus green, then docs + PLAN.md progress entry.

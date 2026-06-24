# Data-Quality Proofing — Design

**Status:** v1 implemented (2026-06-24) — see §11 for what shipped vs. deferred.
**Scope:** `PROC PROOF` (the validation tier). Inline row-level guarding is
recorded as a deferred decision in §9, not part of this design.

This document locks the shape of the feature before any code, following the same
practice as the architecture decision record in [`design.md`](design.md) §14–16.

## 1. Motivation

Analytical systems fail more often on **bad data** than on bad code — and the
failures are silent: a wrong number flows downstream and is trusted. ASS already
proves *its own* correctness by asserting expected dataset values in the
compatibility corpus; **proofing brings that same discipline to the user's
production data** — declare what must be true, verify it, and gate the pipeline
when it isn't.

This is the **first deliberate feature beyond SAS compatibility.** Base SAS has
integrity constraints but no declarative validation PROC, so `PROC PROOF` is an
ASS value-add, not a compat target. That is a conscious shift from
"SAS-compatible subset" toward "SAS-compatible **plus** value-adds," and it fits
the project's audience (regulated industries that need trust, auditability, and
regulatory support).

## 2. The model — three tiers, one shared core

Data quality spans three enforcement points. They share **one assertion
vocabulary and one severity/violation/exit model**; only the enforcement point
differs.

| Tier | What it does | When it runs | Status |
|------|--------------|--------------|--------|
| **Validation** (`PROC PROOF`) | Vet an existing dataset — typically data you *receive* (incoming files, DB extracts, legacy `.sas7bdat`) | On demand, as a step | **This design (v1)** |
| **Inline guarding** (`constrain` / enhanced `error`) | Guard rows *during* the DATA step that produces them; remediate/route in-flight | Per row, inline | Deferred — see §9 |
| **Attached constraints** | Rules bound to a dataset, enforced by *every* writer | Implicitly, on every mutation | Deferred / maybe-never |

v1 anchors on validation because it is self-contained (a read-only PROC, no
surgery on the DATA step runtime), it is the higher-value half for an ETL/
migration tool (the pain is overwhelmingly bad *input* data), and it is the
cleanest adoption wedge.

## 3. `PROC PROOF` — synopsis

```sas
proc proof data=<dataset> [out=<violations>] [maxsample=<n>] [severity=warn|error];
  <assertions>
run;
```

PROC options:

- `data=` — the dataset to validate (WORK, a base libref, or a database libref).
- `out=` — optional dataset to receive the offending rows (see §6).
- `maxsample=` — cap on sampled offending rows captured **per assertion** for the
  report (default 20). Bounds memory regardless of how many rows fail.
- `severity=` — the step-wide default severity (default `error`); individual
  assertions may override it.

## 4. Assertion catalog

Assertions fall into three operational classes by *how* they are checked.

**Polarity convention: every assertion states what must HOLD.** A *violation* is a
row (or a dataset) for which the assertion is false. This matches the natural
reading of the keywords (`range premium >= 0` = "premium is constrained to ≥ 0")
and keeps one mental model across the whole feature.

### Schema — checked from metadata, before any rows are read

```sas
require policy_id premium state;     /* these columns must exist            */
type    premium=num policy_id=char;  /* declared data types must match      */
```

A failed `require` also marks any later assertion referencing the missing column
as "could not run" rather than silently passing.

### Row-local — evaluated in the single row scan

```sas
notnull policy_id premium;               /* values present (not missing)        */
values  state in ("CA" "NY" "TX");       /* domain / allowed set                */
range   age 0 - 120;                      /* inclusive numeric bound (sugar)     */
rule "dates ordered": eff_date <= exp_date;  /* arbitrary boolean per row       */
```

`rule "label": <expression>` is the general escape hatch — any boolean expression
over the row, including cross-column comparisons. `notnull`, `values`, and `range`
are declarative shorthands that also yield better default messages.

### Set-level — need cross-row or cross-table state

```sas
unique policy_id;                          /* no duplicate key combinations     */
key    region references regions(region);  /* referential integrity            */
```

These cannot be expressed as row-local single-row checks and so live only in
`PROC PROOF` (not in any future inline form).

### Per-assertion tail

```sas
range   premium >= 0 / severity=error message="negative premium";
notnull email        / severity=warn;
```

## 5. Outcome & severity model

- Each assertion carries a severity (its own, else the step default, else
  `error`).
- The run accumulates, per assertion: violation count, rows checked, and a capped
  sample of offending rows.
- **Report (listing):** one block per assertion — kind, label, `PASS`/`FAIL`,
  `violations / checked`, and the sampled rows.
- **Process outcome:** if any **error**-level assertion fails → log `ERROR`, the
  step fails, and the CLI exits **non-zero** (this is what lets CI / regulated
  pipelines gate on data quality). **warn**-level failures log `WARNING`, the run
  continues, and the exit code is unaffected.
- **Multi-step programs (default):** a failing proof step sets the run's error
  state but does **not** halt subsequent steps; the process still exits non-zero
  at the end. A future `abort` option can request immediate halt. (Open detail to
  confirm at implementation: whether `abort` is needed in v1.)

## 6. Output: the violations dataset

`out=bad` writes the offending rows for downstream triage/quarantine. Shape:
**one record per (source row × failed rule)** — a row that breaks two rules
produces two records — annotated with:

- the source row's variables (so the bad data is visible),
- `_rule_` — the failing assertion's label/kind,
- `_obs_` — the source observation number.

Per-rule records (rather than one row per source obs with a list) keep the result
trivially filterable: `where _rule_ = "dates ordered"`. Not capped by
`maxsample=` — that cap is only for the in-memory report sample.

## 7. Operational execution

`PROC PROOF` is a **read-only DATA step that emits a verdict instead of a
dataset.** It reuses existing machinery — typed in-memory datasets, the
expression evaluator over a row buffer, the step logger, and the run's error/exit
path. Phases:

1. **Parse** the body into an ordered list of typed assertions.
2. **Bind & schema-check** — resolve column names to indices, compile each `rule`
   expression once, settle `require`/`type` from metadata immediately.
3. **Setup** stateful assertions — allocate the `unique` hash set; load each
   referenced parent table's key column into a hash set.
4. **Single row scan** — for each row, evaluate all row-local assertions and feed
   the stateful ones; on a violation, tally and (under the cap) sample.
5. **Finalize & report** — resolve set-level results (which keys duplicated),
   write the report and the `out=` dataset, set the process outcome.

Cost: ~one linear pass over the target, plus one pass per referenced parent table.
Memory is bounded by the key sets and the capped samples — the same order as
`PROC MEANS`, no extra whole-dataset materialization.

## 8. Phased scope

- **v1:** `require`, `type`, `notnull`, `values`, `range`, `rule`, `unique`,
  `key … references`.
- **Deferred (statistical tier):** distribution expectations, historical
  consistency, anomaly thresholds. These need aggregation/state machinery and
  usually a reference baseline, so they are a separate, later effort.

## 9. Future: inline guarding (open decision, deferred)

Row-level guards *during* a mutating DATA step — to protect the outputs you
produce in-flight and remediate/route bad rows — are valuable but **not in this
scope.** Two candidate designs, decision deferred until `PROC PROOF` is in real
use:

- **(A) A `constrain` statement:** `constrain <invariant> [else <stmt | do…end>];`
  — declarative and reads well, but it is a non-SAS extension and overlaps
  `if/then/do/end`.
- **(B) Enhance SAS's existing `error` statement** to feed the same
  violation/severity/exit model, so `if cond then error "…";` becomes the inline
  guard — 100% SAS-compatible, no second idiom.

Leaning toward **(B)** (keeps programs runnable in real SAS, which matters for the
migration audience) unless (A)'s ergonomics prove worth the divergence once the
shared data-quality model from this design exists.

## 10. Testing / corpus plan

Following the project's corpus discipline, add an item per assertion kind, each
exercising **both** a passing case and a violating case, asserting:

- the `out=` violations dataset values via `expected.datasets` (the primary
  value-verification), and
- the pass/fail outcome (and, where relevant, the non-zero exit).

Because a proof step's product is a verdict plus an optional dataset, the existing
value-verification harness covers it directly through `out=`.

## 11. Implementation status (v1, 2026-06-24)

Implemented (`runtime/proof.go`, parsed in `parser` as `ast.ProofStatement`,
dispatched from `runtime.dispatchProc` because it reuses the DATA-step evaluator
and PDV):

- **Assertions:** `require`, `notnull`, `values … in (…)`, `range <var> lo - hi`
  (inclusive), `rule "label": <expr>` (any boolean expression over the row),
  `unique <vars>` (flags every row in a duplicated key group).
- **PROC options:** `out=`, `maxsample=` (default 20), `severity=` (step default).
- **Per-assertion tail:** `/ severity=warn|error message="…"` — on every assertion
  **except `rule`**, whose expression consumes `/` as division (so a rule's
  severity comes from the step default; see below).
- **Outcome model:** per-assertion report to stdout (PASS/FAIL/N-RUN +
  violations/checked + sampled offending obs); `out=` dataset with one record per
  (source row × failed assertion) annotated `_rule_`/`_obs_`, sorted by
  `(_obs_, _rule_)`; error-level failures log `ERROR` and make the CLI exit
  non-zero **without halting** the program (via `log.Logger.ErrorCount`), warn-level
  failures log `WARNING` and don't affect the exit code. A reference to an unknown
  column is reported as "could not run" rather than failing.
- Reads any `data=` the resolver handles (WORK, dataset options, base/database
  librefs); `out=` is routed through `table.Library.Store` (so it can target an
  external libref too).

Deferred from the v1 catalog in §8:

- **`type`** (declared-type schema check) — not yet parsed/checked.
- **`key … references parent(col)`** (referential integrity) — needs loading the
  parent key set; the headline set-level check (`unique`) is in, this is the next
  set-level addition.
- **`range`'s relational form** (`range premium >= 0`) — use `rule` for relational
  bounds; `range` currently takes the inclusive `lo - hi` form only.
- A **`/` option tail on `rule`** — blocked by the division-operator ambiguity;
  revisit if rule-level severity overrides are wanted (e.g. capture the rule body
  raw and split the tail before parsing the expression).
- **`abort`** (immediate halt) — v1 always continues to the next step and gates via
  the exit code.

Tests: `runtime/proof_test.go` (violations + out= shape + exit semantics, all
pass, warn-only, unique duplicates, values, unknown column) and corpus
`proof_001` (value-verifies the `out=` dataset).

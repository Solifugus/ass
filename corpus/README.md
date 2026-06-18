# Compatibility Corpus

This directory holds the SAS-compatibility test corpus. Each item is a small SAS program
plus metadata and (optionally) expected output, used by the `ass test` harness (Phase 11)
to measure and track compatibility per feature.

See [`FEATURES.md`](FEATURES.md) for the canonical list of feature tags.

## What "compatible" means here

ASS targets **value/result compatibility**, not byte-identical presentation. The bar is:
the output *datasets* have the same columns and values, PROC SQL returns the same result
set, and computed statistics match — because that is what someone migrating SAS programs
or validating results actually depends on. SAS's exact listing spacing and log wording are
**explicitly a non-goal**.

That distinction is what makes the corpus verifiable without a SAS license: SAS's data and
numeric semantics are deterministic, so the expected *values* of a program can be hand-derived
and asserted. Accordingly, the primary correctness signal is **`expected.datasets`** — the
hand-derived contents of one or more output datasets, which the harness compares against the
datasets the program actually produced (numeric values within tolerance, character values
exact, `.`/null = missing).

Three levels of checking, in priority order:

1. **Value compatibility** (`expected.datasets`) — the real bar; hand-derived, verifiable now.
2. **Presentation regression** (`expected_output.txt`) — optional golden file guarding against
   drift in ASS's *own* listing format. It is an ASS baseline, **not** a claim of byte-equality
   with SAS, and is only used when present.
3. **Byte-identical-to-SAS** — out of scope.

## On-disk layout

Each corpus item is its own directory named by a unique `id`:

```
corpus/
  <id>/
    input.sas            # the SAS source program (required)
    meta.yaml            # metadata describing the item (required)
    expected_output.txt  # expected listing/output, if verified (optional)
    expected_log.txt     # expected SAS-style log, if verified (optional)
```

Naming convention for `id` / directory: `<feature>_<subfeature>_<NNN>`, e.g.
`data_step_basic_001`, `proc_sort_byvars_002`. The directory name must equal the `id`
field in `meta.yaml`.

## `meta.yaml` schema

```yaml
id: data_step_basic_001        # must match the directory name
source: hand-written           # origin: hand-written, sas-code-examples, sas-communities, ...
license: MIT                    # license of the sample; hand-written items are MIT (this repo)
features:                       # one or more tags from FEATURES.md
  - data-step
  - input
  - datalines
  - proc-print
expected:
  parse: pass                  # pass | fail  — should the parser accept it?
  execute: pass                # pass | fail | skip — should it run without error?
  output: unverified           # verified | unverified | none — LEGACY byte-listing check
                               #   (only used if expected_output.txt is present; not the bar)
  datasets:                    # PRIMARY check: hand-derived expected values per output dataset
    people:
      columns: [name, age]     # optional: asserts column names AND order (case-insensitive)
      rows:                    # one inner list per observation, in dataset order
        - ["John", 25]         #   numbers compare with tolerance; strings exact; "."/null = missing
        - ["Mary", 30]         #   provide fewer cells than columns to check a prefix of the row
priority: 1                    # 1 = core/early, 2 = secondary, 3 = stretch
notes: |                       # optional free text
  Optional explanation, caveats, or what this item specifically exercises.
```

### Field notes

- **`expected.parse`** — the only field meaningful for `--parse-only` runs. A `fail` here
  marks a negative test (the parser *should* reject it); pair with the `unsupported` tag.
- **`expected.execute: skip`** — use when the item parses but execution isn't implemented
  yet, so the harness counts it as parsed-not-executed rather than a failure.
- **`expected.datasets`** — the primary correctness check. Declare the datasets whose values
  you can hand-derive from SAS semantics; the harness compares them to what the program produced
  and fails the item on any mismatch. Prefer this over `expected_output.txt` for new items.
- **`expected.output`** — the legacy byte-listing comparison. Only checked when `verified` *and*
  an `expected_output.txt` is present; treat it as an ASS-baseline regression guard, not a claim
  of byte-equality with SAS. New items should usually leave it `unverified` and rely on `datasets`.

## Worked example

Directory `corpus/data_step_basic_001/`:

`input.sas`
```sas
data people;
  input name $ age;
  datalines;
John 25
Mary 30
;
run;

proc print data=people;
run;
```

`meta.yaml`
```yaml
id: data_step_basic_001
source: hand-written
license: MIT
features:
  - data-step
  - input
  - datalines
  - proc-print
expected:
  parse: pass
  execute: pass
  output: unverified
  datasets:
    people:
      columns: [name, age]
      rows:
        - ["John", 25]
        - ["Mary", 30]
priority: 1
notes: |
  Canonical minimal DATA step + PROC PRINT (design doc milestones 3 and 4).
```

The `datasets` block asserts that the program builds `people` with columns `name, age` and the
two expected observations — a value check that holds regardless of how PROC PRINT spaces its
listing. An `expected_output.txt` may still be added as an ASS-baseline regression guard, but it
is optional and is **not** treated as a byte-for-byte SAS equivalence claim.

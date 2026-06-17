# Compatibility Corpus

This directory holds the SAS-compatibility test corpus. Each item is a small SAS program
plus metadata and (optionally) expected output, used by the `ass test` harness (Phase 11)
to measure and track compatibility per feature.

See [`FEATURES.md`](FEATURES.md) for the canonical list of feature tags.

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
  output: verified             # verified | unverified | none
                               #   verified   = expected_output.txt is known-correct (hand-derived from SAS)
                               #   unverified = expected_output.txt absent or not confirmed against SAS
                               #   none       = no output expected (e.g. parse-only items)
priority: 1                    # 1 = core/early, 2 = secondary, 3 = stretch
notes: |                       # optional free text
  Optional explanation, caveats, or what this item specifically exercises.
```

### Field notes

- **`expected.parse`** — the only field meaningful for `--parse-only` runs. A `fail` here
  marks a negative test (the parser *should* reject it); pair with the `unsupported` tag.
- **`expected.execute: skip`** — use when the item parses but execution isn't implemented
  yet, so the harness counts it as parsed-not-executed rather than a failure.
- **`expected.output: unverified`** — set this when you cannot confidently hand-derive the
  exact SAS output. The harness will not fail the item on output mismatch, but will report
  it as unverified so it can be confirmed later against real SAS.

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
  output: verified
priority: 1
notes: |
  Canonical minimal DATA step + PROC PRINT (design doc milestones 3 and 4).
```

`expected_output.txt`
```
Obs    name    age

  1    John     25
  2    Mary     30
```

> The exact column widths/spacing in `expected_output.txt` are defined by the PROC PRINT
> renderer (Phase 5). Until that renderer is finalized, mark such items
> `expected.output: unverified` and tighten them once the renderer is stable.

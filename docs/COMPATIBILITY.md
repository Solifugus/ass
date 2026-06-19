# Compatibility matrix

Generated from the compatibility corpus via `ass test corpus/` (2026-06-19).
Each percentage is the share of corpus items tagged with a feature that pass
(parse + execute as the item's `meta.yaml` expects). Regenerate with:

```bash
ass test corpus/
```

## What compatibility means

ASS targets **value/result compatibility** with SAS, not byte-identical output:

- **The goal** — produced datasets have the same columns and values, PROC SQL returns the
  same result set, computed statistics match (within numeric tolerance). This is what a SAS
  migration or a results validation actually depends on, and — because SAS's data semantics
  are deterministic — it is verifiable by hand-deriving expected values, no SAS license
  required. Corpus items assert this via `expected.datasets` (see [`../corpus/README.md`](../corpus/README.md)).
- **Best-effort** — listing/report layout is readable and stable. The harness can guard ASS's
  *own* listing format against drift via an optional `expected_output.txt` golden file, treated
  as an ASS baseline.
- **A non-goal** — byte-for-byte identical PROC listings and log wording versus real SAS.

Where a procedure's *values* would differ from SAS by convention rather than by error (e.g. a
regression parameterization), that is called out explicitly below rather than hidden behind a
passing cosmetic check.

## Overall

| Metric | Result |
|--------|--------|
| Items | 34 |
| Parsed | 34 (100.0%) |
| Executed | 34 (100.0%) |
| Passed | 34 (100.0%) |
| Value-verified | 8 items assert dataset values; all match |

## Per-feature

| Feature | Pass/Total | % |
|---------|-----------|---|
| arrays | 1/1 | 100.0% |
| assignment | 4/4 | 100.0% |
| automatic-vars | 1/1 | 100.0% |
| by-group | 2/2 | 100.0% |
| class | 1/1 | 100.0% |
| data-step | 28/28 | 100.0% |
| dataset-options | 1/1 | 100.0% |
| datalines | 5/5 | 100.0% |
| do-loop | 2/2 | 100.0% |
| expressions | 2/2 | 100.0% |
| file-put | 1/1 | 100.0% |
| formats | 3/3 | 100.0% |
| if-then-else | 3/3 | 100.0% |
| infile | 2/2 | 100.0% |
| informats | 1/1 | 100.0% |
| input | 5/5 | 100.0% |
| macro-control | 1/1 | 100.0% |
| macro-def | 2/2 | 100.0% |
| macro-let | 1/1 | 100.0% |
| macro-var | 3/3 | 100.0% |
| merge | 1/1 | 100.0% |
| proc-freq | 2/2 | 100.0% |
| proc-glm | 1/1 | 100.0% |
| proc-means | 1/1 | 100.0% |
| proc-print | 21/21 | 100.0% |
| proc-reg | 1/1 | 100.0% |
| proc-sort | 3/3 | 100.0% |
| proc-sql | 4/4 | 100.0% |
| retain | 2/2 | 100.0% |
| set | 4/4 | 100.0% |
| sql-create-table | 1/1 | 100.0% |
| sql-groupby | 1/1 | 100.0% |
| sql-join | 1/1 | 100.0% |
| sql-select | 4/4 | 100.0% |
| sum-statement | 2/2 | 100.0% |
| user-formats | 1/1 | 100.0% |
| where | 1/1 | 100.0% |

> 100% means every corpus item *currently authored* for a feature passes — it is
> a regression signal, not a claim of full SAS coverage. Coverage grows by adding
> corpus items (see [`CONTRIBUTING.md`](CONTRIBUTING.md)). Output is compared by parse/execute
> success; byte-level output verification against real SAS is pending (corpus
> items are marked `output: unverified`).

## Known unsupported / deferred constructs

- PROC FREQ n-way (3+) tables, `/ options` (nocol/norow/chisq), and association statistics (one- and two-way tables are supported)
- `proc format` PICTURE/INVALUE statements and on-disk format catalogs (VALUE formats are supported); user formats are applied in PROC PRINT (not yet in MEANS/FREQ/SQL output)
- Column/pointer input (`input name $ 1-10 age 11-13;`, `@`/`#`) and time/datetime informats (list-input informats such as `comma`/`dollar`/`date9`/`mmddyy` are supported); `'..'t`/`'..'dt` time/datetime literals
- Dataset options `firstobs=`/`obs=`, numbered var-list ranges in `keep=`/`drop=` (e.g. `keep=x1-x5`), and options on PROC `out=` (`keep=`/`drop=`/`rename=`/`where=` on SET/MERGE/DATA/PROC `data=` are supported)
- PROC GLM with SAS's generalized-inverse (sweep) parameterization, Type I/III SS, F tests, and LSMEANS/CONTRAST/ESTIMATE. CLASS effects **are** supported via **reference-cell coding** (k−1 indicators, last level = reference at estimate 0) — numerically correct for the fit, predictions, and level-vs-reference differences, but the intercept and per-level estimates **differ from SAS by convention** (SAS keeps all levels and flags the aliased one "Biased"). This is a deliberate, documented divergence; the design→solve seam allows a future sweep-based upgrade when a real-SAS reference is available.
- Flat-file **reading** via `infile "<path>"` + list `input` is supported, including `dlm=`/`delimiter=`, `dsd` (CSV-style quoted fields, embedded delimiters, consecutive-delimiter missings), `firstobs=`, and `obs=`. Flat-file **writing** via `file "<path>"` + `put` is supported, including `dlm=`/`dsd` (the delimiter joins items; DSD quotes values containing the delimiter or a quote), string literals, inline/associated formats, and `data _null_` (a side-effect-only step that creates no dataset). Note: a missing numeric writes as `.` and a missing character as empty, as the DATA step `put` does (not the empty-field convention of PROC EXPORT). Not yet: `PROC IMPORT`/`PROC EXPORT`, column/pointer input and output (`@`/`#`, fixed-width ranges), `put _all_`/named output (`var=`), and `infile`/`file` options beyond the above (e.g. `lrecl=`, `pad`, `end=`, `mod`, multi-line `/`).
- External-database LIBNAME engines are supported **read-only** for Postgres, SQL Server, and Oracle (`libname pg postgres "…"; … set pg.table;`). Not yet: writing datasets back to a database, implicit query pushdown (ASS reads the table and computes locally — same results, full transfer), PROC SQL pass-through to external librefs, DB2 (needs the CGo IBM CLI driver), and the long tail of SAS/ACCESS options. See [`databases.md`](databases.md).
- JSON harness output (machine-readable report); SAS-byte-identical listing comparison (a non-goal — see "What compatibility means" above; value comparison via `expected.datasets` is the supported mechanism)

See [`../corpus/FEATURES.md`](../corpus/FEATURES.md) for the full feature-tag catalog and intended levels.

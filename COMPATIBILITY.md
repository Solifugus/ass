# Compatibility matrix

Generated from the compatibility corpus via `ass test corpus/` (2026-06-17).
Each percentage is the share of corpus items tagged with a feature that pass
(parse + execute as the item's `meta.yaml` expects). Regenerate with:

```bash
ass test corpus/
```

## Overall

| Metric | Result |
|--------|--------|
| Items | 26 |
| Parsed | 26 (100.0%) |
| Executed | 26 (100.0%) |
| Passed | 26 (100.0%) |

## Per-feature

| Feature | Pass/Total | % |
|---------|-----------|---|
| arrays | 1/1 | 100.0% |
| assignment | 4/4 | 100.0% |
| automatic-vars | 1/1 | 100.0% |
| by-group | 2/2 | 100.0% |
| data-step | 26/26 | 100.0% |
| dataset-options | 1/1 | 100.0% |
| datalines | 5/5 | 100.0% |
| do-loop | 2/2 | 100.0% |
| expressions | 2/2 | 100.0% |
| formats | 3/3 | 100.0% |
| if-then-else | 3/3 | 100.0% |
| informats | 1/1 | 100.0% |
| input | 5/5 | 100.0% |
| macro-control | 1/1 | 100.0% |
| macro-def | 2/2 | 100.0% |
| macro-let | 1/1 | 100.0% |
| macro-var | 3/3 | 100.0% |
| merge | 1/1 | 100.0% |
| proc-freq | 2/2 | 100.0% |
| proc-means | 1/1 | 100.0% |
| proc-print | 19/19 | 100.0% |
| proc-reg | 1/1 | 100.0% |
| proc-sort | 3/3 | 100.0% |
| proc-sql | 4/4 | 100.0% |
| retain | 2/2 | 100.0% |
| set | 3/3 | 100.0% |
| sql-create-table | 1/1 | 100.0% |
| sql-groupby | 1/1 | 100.0% |
| sql-join | 1/1 | 100.0% |
| sql-select | 4/4 | 100.0% |
| sum-statement | 2/2 | 100.0% |
| user-formats | 1/1 | 100.0% |
| where | 1/1 | 100.0% |

> 100% means every corpus item *currently authored* for a feature passes — it is
> a regression signal, not a claim of full SAS coverage. Coverage grows by adding
> corpus items (see `CONTRIBUTING.md`). Output is compared by parse/execute
> success; byte-level output verification against real SAS is pending (corpus
> items are marked `output: unverified`).

## Known unsupported / deferred constructs

- PROC FREQ n-way (3+) tables, `/ options` (nocol/norow/chisq), and association statistics (one- and two-way tables are supported)
- `proc format` PICTURE/INVALUE statements and on-disk format catalogs (VALUE formats are supported); user formats are applied in PROC PRINT (not yet in MEANS/FREQ/SQL output)
- Column/pointer input (`input name $ 1-10 age 11-13;`, `@`/`#`) and time/datetime informats (list-input informats such as `comma`/`dollar`/`date9`/`mmddyy` are supported); `'..'t`/`'..'dt` time/datetime literals
- Dataset options `firstobs=`/`obs=`, numbered var-list ranges in `keep=`/`drop=` (e.g. `keep=x1-x5`), and options on PROC `out=` (`keep=`/`drop=`/`rename=`/`where=` on SET/MERGE/DATA/PROC `data=` are supported)
- PROC GLM CLASS effects / design-matrix coding (PROC REG/GLM OLS estimates, std-err, t-value, `Pr>|t|`, and R² are supported)
- `--compare-output` / JSON harness output (tied to SAS-verified expected files)

See `corpus/FEATURES.md` for the full feature-tag catalog and intended levels.

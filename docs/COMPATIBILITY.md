# Compatibility matrix

Generated from the compatibility corpus via `ass test corpus/` (2026-06-20; .xlsx import/export).
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
| Items | 58 |
| Parsed | 58 (100.0%) |
| Executed | 58 (100.0%) |
| Passed | 58 (100.0%) |
| Value-verified | 30 items assert dataset values; all match |

## Per-feature

| Feature | Pass/Total | % |
|---------|-----------|---|
| arrays | 1/1 | 100.0% |
| assignment | 4/4 | 100.0% |
| automatic-vars | 1/1 | 100.0% |
| by-group | 2/2 | 100.0% |
| class | 1/1 | 100.0% |
| data-step | 49/49 | 100.0% |
| dataset-options | 3/3 | 100.0% |
| datalines | 10/10 | 100.0% |
| do-loop | 2/2 | 100.0% |
| expressions | 2/2 | 100.0% |
| file-put | 5/5 | 100.0% |
| formats | 4/4 | 100.0% |
| labels | 1/1 | 100.0% |
| if-then-else | 4/4 | 100.0% |
| infile | 5/5 | 100.0% |
| informats | 4/4 | 100.0% |
| input | 11/11 | 100.0% |
| libname | 7/7 | 100.0% |
| line-hold | 4/4 | 100.0% |
| macro-control | 1/1 | 100.0% |
| macro-def | 2/2 | 100.0% |
| macro-let | 1/1 | 100.0% |
| macro-var | 3/3 | 100.0% |
| merge | 1/1 | 100.0% |
| proc-append | 1/1 | 100.0% |
| proc-export | 2/2 | 100.0% |
| proc-freq | 4/4 | 100.0% |
| proc-glm | 1/1 | 100.0% |
| proc-import | 2/2 | 100.0% |
| proc-means | 1/1 | 100.0% |
| proc-print | 42/42 | 100.0% |
| proc-reg | 1/1 | 100.0% |
| proc-sort | 4/4 | 100.0% |
| proc-sql | 7/7 | 100.0% |
| query-pushdown | 1/1 | 100.0% |
| retain | 2/2 | 100.0% |
| sas7bdat | 2/2 | 100.0% |
| set | 13/13 | 100.0% |
| sql-create-table | 2/2 | 100.0% |
| sql-external-source | 1/1 | 100.0% |
| sql-groupby | 2/2 | 100.0% |
| sql-join | 2/2 | 100.0% |
| sql-passthrough | 1/1 | 100.0% |
| sql-select | 4/4 | 100.0% |
| sum-statement | 2/2 | 100.0% |
| user-formats | 2/2 | 100.0% |
| where | 4/4 | 100.0% |

> 100% means every corpus item *currently authored* for a feature passes — it is
> a regression signal, not a claim of full SAS coverage. Coverage grows by adding
> corpus items (see [`CONTRIBUTING.md`](CONTRIBUTING.md)). Output is compared by parse/execute
> success; byte-level output verification against real SAS is pending (corpus
> items are marked `output: unverified`).

## Date/time functions

**Date/time functions are supported** (operating on the SAS encodings — date =
days since 1960-01-01, datetime = seconds since then, time = seconds since
midnight): `today`/`date`, `datetime`, `time`, `mdy` (returns missing for invalid
dates like 30FEB), `year`/`month`/`day`/`qtr`/`weekday` (Sunday=1), `datepart`/
`timepart`, `hms`/`dhms`, `intck` and `intnx` over the intervals
`day`/`week`/`month`/`qtr`/`semiyear`/`year`/`hour`/`minute`/`second`, including
multipliers (`month2`), shifts (`week.2`), and `dt`-prefixed datetime intervals
(`dtday`, `dtmonth`) — `intnx` alignments `b`/`m`/`e`/`s`. Missing arguments
propagate.

## Value-adds beyond SAS

These are intentional ASS features with **no Base SAS equivalent** — clean-room
additions, not compatibility targets.

- **PROC PROOF** — declarative data-quality validation (the full §8 validation
  catalog). Assert `require`, `type <var>=num|char`, `notnull`, `values … in (…)`,
  `range <var> lo - hi` or relational `range <var> >= <num>`, `unique <vars>`,
  `key <cols> references <parent>(<cols>)` (single/multi-column referential
  integrity), and `rule "label": <expr>` over a dataset; each takes an optional
  `/ severity=warn|error message="…"` tail (rule included). Produces a report, an
  optional `out=` violations dataset (one record per source-row × failed
  assertion, annotated `_rule_`/`_obs_`), and a **non-zero process exit** when an
  error-level assertion fails (without halting the run) so CI / regulated
  pipelines can gate on data quality. Deferred: `abort` (fail-fast) and the
  statistical tier. See [`proofing.md`](proofing.md).

## Known unsupported / deferred constructs

- PROC FREQ: one- and two-way tables, **n-way (3+) via `/ list`** (one row per distinct combination), the **`/ options`** nocol/norow/nopercent/nofreq/nocum (suppress parts of the table) and **`/ chisq`** (Pearson chi-square statistic, DF, and p-value for a two-way table) are supported. Not yet: the default (non-`list`) stratified n-way layout, and association statistics beyond Pearson chi-square (likelihood-ratio, Fisher exact, measures of association)
- `proc format` **VALUE, INVALUE, and PICTURE** statements are supported (on-disk format catalogs are not). VALUE formats are applied in PROC PRINT and used for **grouping in PROC FREQ and PROC MEANS/SUMMARY** (a user VALUE format collapses underlying values into one formatted category/class level, matching SAS). **INVALUE** defines a user informat (`invalue grade 'A'=4 'B'=3 other=0;`, `invalue $resp 'Y'='Yes';`, numeric-range keys `1-10=1`) that INPUT applies to read input into mapped numeric/character values. **PICTURE** defines an output template of digit selectors (zero-suppressing `0` vs forcing nonzero) and message characters, with `prefix=`/`mult=`/`fill=` options, applied in PROC PRINT and via `put()`. Not yet: user formats on PROC SQL output columns.
- The `label <var>="text";` statement is supported in the DATA step and in PROC steps (e.g. PROC PRINT). DATA-step labels become permanent column metadata and are inherited through SET/MERGE (an explicit `label` in a later step overrides); `proc print ... label` renders labels as column headers, and a `label` statement inside the step overrides the stored label for that listing. `label` is correctly disambiguated as a variable name when used as `label = expr` (it is not a reserved word). Not yet: SAS's multi-line header wrapping of long labels (ASS prints each label on one header line — a presentation detail, not a value difference).
- **Time/datetime informats and `'..'t`/`'..'dt` literals are supported**: `'14:30:00't` is a SAS time (seconds since midnight), `'01JAN2020:14:30:00'dt` a SAS datetime (seconds since 1960-01-01 00:00:00); the `TIMEw.`/`DATETIMEw.` informats read those forms and the matching output formats render them. Multi-line **`#n` line pointers** are supported on both INPUT and PUT (`input #1 a #2 b;` reads one observation across physical lines; `put #1 a #2 b;` writes one observation as several lines); column input `input name $ 1-10 age 11-13;`/`@n`/`+n` and column output `put name $ 1-10;`/`@n`/`+n` are supported; **trailing `@`/`@@` line-hold on INPUT and PUT is supported** — on INPUT `@@` reads several observations from one line across iterations and `@` holds the line within the iteration; on PUT the same modifiers hold the output line so several PUTs (or observations) build one physical line; list-input informats such as `comma`/`dollar`/`date9`/`mmddyy` are supported. **Combining `#n` with a trailing `@`/`@@` hold is supported** — `input a #2 b @@;` holds the multi-line record group across iterations with a separate column cursor per line, reading several observations from one group (semantics hand-derived from documented SAS pointer behavior)
- Options on PROC `out=`. Supported dataset options on SET/MERGE/DATA/PROC `data=`: `keep=`/`drop=`/`rename=`/`where=` and **`firstobs=`/`obs=`** (positional observation range — `firstobs=` first observation, `obs=` last-observation number, applied before WHERE). **Numbered var-list ranges** (`keep=x1-x5`, `keep x1-x5;`, `drop=x2-x3`) expand to the enumerated names in both the dataset-option and statement forms, zero-padded to the low endpoint's digit width
- PROC GLM with SAS's generalized-inverse (sweep) parameterization, Type I/III SS, F tests, and LSMEANS/CONTRAST/ESTIMATE. CLASS effects **are** supported via **reference-cell coding** (k−1 indicators, last level = reference at estimate 0) — numerically correct for the fit, predictions, and level-vs-reference differences, but the intercept and per-level estimates **differ from SAS by convention** (SAS keeps all levels and flags the aliased one "Biased"). This is a deliberate, documented divergence; the design→solve seam allows a future sweep-based upgrade when a real-SAS reference is available.
- Flat-file **reading** via `infile "<path>"` + list `input` is supported, including `dlm=`/`delimiter=`, `dsd` (CSV-style quoted fields, embedded delimiters, consecutive-delimiter missings), `firstobs=`, and `obs=`. Flat-file **writing** via `file "<path>"` + `put` is supported, including `dlm=`/`dsd` (the delimiter joins items; DSD quotes values containing the delimiter or a quote), string literals, inline/associated formats, and `data _null_` (a side-effect-only step that creates no dataset). Note: a missing numeric writes as `.` and a missing character as empty, as the DATA step `put` does (not the empty-field convention of PROC EXPORT). `PROC IMPORT`/`PROC EXPORT` handle delimited files (`dbms=csv`/`tab`/`dlm`) **and `.xlsx` workbooks (`dbms=xlsx`)**: IMPORT reads the header for column names (`getnames=`, default yes), honors `datarow=` and `delimiter=`/`dlm=`, and sniffs each column's type (numeric if every non-empty value parses, else character); EXPORT writes a header row (`putnames=`, default yes), DSD-quotes values containing the delimiter/quote, and writes a missing value as an empty field. The `.xlsx` reader/writer is a dependency-free single-worksheet implementation (Go `archive/zip` + `encoding/xml`; inline strings on write, shared-strings resolved on read). **Column/pointer input and output** are supported: column input reads each variable from a 1-based column range (`input id 1-3 name $ 5-14 age 16-17;`), formatted input reads an informat width from the column pointer, and `@n`/`+n` move the pointer; column output positions each `put` item by range or pointer (`put name $ 1-10 age 11-13;`, `put @5 label $ @10 id 3.;`), left-justifying character and right-justifying numeric values within an explicit range. Trailing `@`/`@@` line-hold on INPUT **and PUT** is supported (see above): on PUT, `@` holds the output line within the iteration (released automatically at the iteration boundary) and `@@` holds it across iterations (released by a PUT without a trailing hold, or at end of step), so several PUTs — or several observations — build one physical line; list segments join by the FILE separator and column/pointer segments overlay at their absolute columns. **Multi-line `#n` line pointers are supported on both INPUT and PUT** — `#n` reads/writes the n-th physical line of one observation (input reads an observation spread over several lines; output writes an observation as several lines), resetting the column pointer to 1 at each `#n`. **Named output and `_all_` are supported**: `put var=;` writes `var=value`, and `put _all_;` writes every PDV variable (including the automatic `_n_`/`_error_`) as `name=value`. **Combining a `#n` line pointer with a trailing `@`/`@@` hold on INPUT is supported** (the multi-line record group is held across iterations with a per-line column cursor). Not yet: `infile`/`file` options beyond the above (e.g. `lrecl=`, `pad`, `end=`, `mod`); `.xlsx` import/export is supported via `dbms=xlsx` (single worksheet), and native `.sas7bdat` files are read via the base LIBNAME engine — see below.
- Native SAS dataset files (`.sas7bdat`) are read through a **base/directory LIBNAME engine**: `libname lib "/dir"; … set lib.member;` (or `proc print data=lib.member;`) reads `dir/member.sas7bdat`. The reader is a clean-room implementation from the public reverse-engineering literature on the format (the layout documented by the ReadStat and sas7bdat open-source projects and Matthew Shotwell's published format notes) — never from proprietary SAS documentation, source, or internals. Supported: 32-bit and 64-bit **little-endian** files; both **row-compression** schemes — `SASYZCRL` (RLE) and `SASYZCR2` (RDC, Ross Data Compression) — as well as uncompressed; numeric (including SAS's truncated <8-byte numerics) and character columns; SAS date/datetime values (stored as numeric days/seconds from 1960-01-01); and column metadata (names, lengths, formats, labels — labels/formats for 32-bit files; on 64-bit files the values read correctly but format/label recovery is skipped). The decompressors are clean-room ports of the public reverse-engineering literature (ReadStat's RLE command table, the sas7bdat R-package vignette, and Ed Ross's published RDC algorithm). Not yet: **big-endian** files (written on legacy big-endian platforms), writing `.sas7bdat`, and on-disk format catalogs. Read-only.
- External-database LIBNAME engines are supported for **reading and writing** with Postgres, SQL Server, Oracle, SQLite, and DB2 (`libname pg postgres "…"; … set pg.table;` to read; `data pg.out; set …;` to write — the target table is dropped and recreated in one transaction, SAS replace semantics; SAS→DB types map character→`VARCHAR`/`NVARCHAR`/`VARCHAR2`, numeric→the engine's double, and date/datetime-formatted numerics→`DATE`/`TIMESTAMP`). **PROC output can also target an external libref**: `proc sort data=work out=db.sorted;` and `proc sql; create table db.totals as select …;` write their results to the bound engine (same replace semantics) via the shared `Library.Store` routing point. **PROC APPEND can append to an external table in place**: `proc append base=db.fact data=work.daily;` performs an INSERT-only load (the table and its existing rows are untouched, not dropped/recreated), creating BASE= from DATA= on the first append, via the shared `Library.Append` routing point (`AppendBackend`); `FORCE` handles variable mismatches as in SAS. The SQLite engine (single file or `:memory:`) and the Postgres/SQL Server/Oracle/SQLite read+write paths are **pure Go** (the `modernc.org/sqlite` driver and pure-Go DB drivers), so they build with `CGO_ENABLED=0` like the rest of the engine; only the **DB2** engine (IBM Db2 LUW, via `ibmdb/go_ibm_db`) needs CGo plus IBM's native CLI driver and so is gated behind a `db2` build tag (`go build -tags db2`) to keep the default build free of it — see [`databases.md`](databases.md). **PROC SQL explicit pass-through** sends a database its own native SQL: `connect to <engine> (connection="…")` (or reuse of an assigned libref), `select … from connection to <engine> (<native query>)` returning the result set as a dataset, `execute (<native sql>) by <engine>` for remote DDL/DML, and `drop table <libref>.<member>` routed to the external table — via `table.SQLBackend`/`table.DropBackend`, supported on every database engine. **Implicit query pushdown** sends a value-safe subset of dataset options to the database so it returns less data: `keep=` becomes a column projection, and a `where=` of numeric comparisons using `=`/`>`/`>=` (which exclude missing in SAS exactly as SQL excludes NULL) is pushed as a SQL `WHERE` — everything else (`<`/`<=`/`ne`, which keep missing in SAS; string/function predicates; `drop=`) is filtered locally, so results are always identical to a full read (`table.FilterBackend` → `dbio.Backend.LoadFiltered`). **Ordinary PROC SQL can also read an external libref as a query source** (not only via pass-through): `select … from db.orders`, including a join of an external table with a WORK table, loads the libref-qualified member on demand into the in-process SQLite engine (WORK-qualified sources resolve too); the load is value-only — the table is read in full and the join/aggregation runs locally. Not yet: pushing joins/aggregation *to the database*, `obs=`/`firstobs=`, and the long tail of SAS/ACCESS options. See [`databases.md`](databases.md).
- SAS-byte-identical listing comparison (a non-goal — see "What compatibility means" above; value comparison via `expected.datasets` is the supported mechanism). JSON harness output **is** supported: `ass test --json corpus/` emits a machine-readable report (summary, per-feature counts, per-item results)

See [`../corpus/FEATURES.md`](../corpus/FEATURES.md) for the full feature-tag catalog and intended levels.

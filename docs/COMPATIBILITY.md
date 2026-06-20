# Compatibility matrix

Generated from the compatibility corpus via `ass test corpus/` (2026-06-20).
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
| Items | 46 |
| Parsed | 46 (100.0%) |
| Executed | 46 (100.0%) |
| Passed | 46 (100.0%) |
| Value-verified | 20 items assert dataset values; all match |

## Per-feature

| Feature | Pass/Total | % |
|---------|-----------|---|
| arrays | 1/1 | 100.0% |
| assignment | 4/4 | 100.0% |
| automatic-vars | 1/1 | 100.0% |
| by-group | 2/2 | 100.0% |
| class | 1/1 | 100.0% |
| data-step | 40/40 | 100.0% |
| dataset-options | 2/2 | 100.0% |
| datalines | 7/7 | 100.0% |
| do-loop | 2/2 | 100.0% |
| expressions | 2/2 | 100.0% |
| file-put | 2/2 | 100.0% |
| formats | 4/4 | 100.0% |
| labels | 1/1 | 100.0% |
| if-then-else | 4/4 | 100.0% |
| infile | 2/2 | 100.0% |
| informats | 2/2 | 100.0% |
| input | 8/8 | 100.0% |
| libname | 6/6 | 100.0% |
| line-hold | 1/1 | 100.0% |
| macro-control | 1/1 | 100.0% |
| macro-def | 2/2 | 100.0% |
| macro-let | 1/1 | 100.0% |
| macro-var | 3/3 | 100.0% |
| merge | 1/1 | 100.0% |
| proc-append | 1/1 | 100.0% |
| proc-export | 1/1 | 100.0% |
| proc-freq | 2/2 | 100.0% |
| proc-glm | 1/1 | 100.0% |
| proc-import | 1/1 | 100.0% |
| proc-means | 1/1 | 100.0% |
| proc-print | 33/33 | 100.0% |
| proc-reg | 1/1 | 100.0% |
| proc-sort | 4/4 | 100.0% |
| proc-sql | 6/6 | 100.0% |
| query-pushdown | 1/1 | 100.0% |
| retain | 2/2 | 100.0% |
| sas7bdat | 2/2 | 100.0% |
| set | 10/10 | 100.0% |
| sql-create-table | 2/2 | 100.0% |
| sql-groupby | 2/2 | 100.0% |
| sql-join | 1/1 | 100.0% |
| sql-passthrough | 1/1 | 100.0% |
| sql-select | 4/4 | 100.0% |
| sum-statement | 2/2 | 100.0% |
| user-formats | 1/1 | 100.0% |
| where | 3/3 | 100.0% |

> 100% means every corpus item *currently authored* for a feature passes — it is
> a regression signal, not a claim of full SAS coverage. Coverage grows by adding
> corpus items (see [`CONTRIBUTING.md`](CONTRIBUTING.md)). Output is compared by parse/execute
> success; byte-level output verification against real SAS is pending (corpus
> items are marked `output: unverified`).

## Known unsupported / deferred constructs

- PROC FREQ n-way (3+) tables, `/ options` (nocol/norow/chisq), and association statistics (one- and two-way tables are supported)
- `proc format` PICTURE/INVALUE statements and on-disk format catalogs (VALUE formats are supported); user formats are applied in PROC PRINT (not yet in MEANS/FREQ/SQL output)
- The `label <var>="text";` statement is supported in the DATA step and in PROC steps (e.g. PROC PRINT). DATA-step labels become permanent column metadata and are inherited through SET/MERGE (an explicit `label` in a later step overrides); `proc print ... label` renders labels as column headers, and a `label` statement inside the step overrides the stored label for that listing. `label` is correctly disambiguated as a variable name when used as `label = expr` (it is not a reserved word). Not yet: SAS's multi-line header wrapping of long labels (ASS prints each label on one header line — a presentation detail, not a value difference).
- Multi-line input/output (`#n` line pointers) and time/datetime informats (column input `input name $ 1-10 age 11-13;`/`@n`/`+n` and column output `put name $ 1-10;`/`@n`/`+n` are supported; **trailing `@`/`@@` line-hold on INPUT is supported** — `@@` reads several observations from one line across iterations, `@` holds the line within the iteration; list-input informats such as `comma`/`dollar`/`date9`/`mmddyy` are supported); `'..'t`/`'..'dt` time/datetime literals
- Dataset options `firstobs=`/`obs=`, numbered var-list ranges in `keep=`/`drop=` (e.g. `keep=x1-x5`), and options on PROC `out=` (`keep=`/`drop=`/`rename=`/`where=` on SET/MERGE/DATA/PROC `data=` are supported)
- PROC GLM with SAS's generalized-inverse (sweep) parameterization, Type I/III SS, F tests, and LSMEANS/CONTRAST/ESTIMATE. CLASS effects **are** supported via **reference-cell coding** (k−1 indicators, last level = reference at estimate 0) — numerically correct for the fit, predictions, and level-vs-reference differences, but the intercept and per-level estimates **differ from SAS by convention** (SAS keeps all levels and flags the aliased one "Biased"). This is a deliberate, documented divergence; the design→solve seam allows a future sweep-based upgrade when a real-SAS reference is available.
- Flat-file **reading** via `infile "<path>"` + list `input` is supported, including `dlm=`/`delimiter=`, `dsd` (CSV-style quoted fields, embedded delimiters, consecutive-delimiter missings), `firstobs=`, and `obs=`. Flat-file **writing** via `file "<path>"` + `put` is supported, including `dlm=`/`dsd` (the delimiter joins items; DSD quotes values containing the delimiter or a quote), string literals, inline/associated formats, and `data _null_` (a side-effect-only step that creates no dataset). Note: a missing numeric writes as `.` and a missing character as empty, as the DATA step `put` does (not the empty-field convention of PROC EXPORT). `PROC IMPORT`/`PROC EXPORT` handle delimited files (`dbms=csv`/`tab`/`dlm`): IMPORT reads the header for column names (`getnames=`, default yes), honors `datarow=` and `delimiter=`/`dlm=`, and sniffs each column's type (numeric if every non-empty value parses, else character); EXPORT writes a header row (`putnames=`, default yes), DSD-quotes values containing the delimiter/quote, and writes a missing value as an empty field. **Column/pointer input and output** are supported: column input reads each variable from a 1-based column range (`input id 1-3 name $ 5-14 age 16-17;`), formatted input reads an informat width from the column pointer, and `@n`/`+n` move the pointer; column output positions each `put` item by range or pointer (`put name $ 1-10 age 11-13;`, `put @5 label $ @10 id 3.;`), left-justifying character and right-justifying numeric values within an explicit range. Trailing `@`/`@@` line-hold on INPUT is supported (see above). Not yet: `put _all_`/named output (`var=`), multi-line `#n` line pointers (input and output) and trailing `@`/`@@` *output* hold, `infile`/`file` options beyond the above (e.g. `lrecl=`, `pad`, `end=`, `mod`), and non-delimited `PROC IMPORT`/`PROC EXPORT` targets such as `.xlsx` (native `.sas7bdat` files are read via the base LIBNAME engine — see below).
- Native SAS dataset files (`.sas7bdat`) are read through a **base/directory LIBNAME engine**: `libname lib "/dir"; … set lib.member;` (or `proc print data=lib.member;`) reads `dir/member.sas7bdat`. The reader is a clean-room implementation from the public reverse-engineering literature on the format (the layout documented by the ReadStat and sas7bdat open-source projects and Matthew Shotwell's published format notes) — never from proprietary SAS documentation, source, or internals. Supported: 32-bit and 64-bit **little-endian** files; both **row-compression** schemes — `SASYZCRL` (RLE) and `SASYZCR2` (RDC, Ross Data Compression) — as well as uncompressed; numeric (including SAS's truncated <8-byte numerics) and character columns; SAS date/datetime values (stored as numeric days/seconds from 1960-01-01); and column metadata (names, lengths, formats, labels — labels/formats for 32-bit files; on 64-bit files the values read correctly but format/label recovery is skipped). The decompressors are clean-room ports of the public reverse-engineering literature (ReadStat's RLE command table, the sas7bdat R-package vignette, and Ed Ross's published RDC algorithm). Not yet: **big-endian** files (written on legacy big-endian platforms), writing `.sas7bdat`, and on-disk format catalogs. Read-only.
- External-database LIBNAME engines are supported for **reading and writing** with Postgres, SQL Server, Oracle, and SQLite (`libname pg postgres "…"; … set pg.table;` to read; `data pg.out; set …;` to write — the target table is dropped and recreated in one transaction, SAS replace semantics; SAS→DB types map character→`VARCHAR`/`NVARCHAR`/`VARCHAR2`, numeric→the engine's double, and date/datetime-formatted numerics→`DATE`/`TIMESTAMP`). **PROC output can also target an external libref**: `proc sort data=work out=db.sorted;` and `proc sql; create table db.totals as select …;` write their results to the bound engine (same replace semantics) via the shared `Library.Store` routing point. **PROC APPEND can append to an external table in place**: `proc append base=db.fact data=work.daily;` performs an INSERT-only load (the table and its existing rows are untouched, not dropped/recreated), creating BASE= from DATA= on the first append, via the shared `Library.Append` routing point (`AppendBackend`); `FORCE` handles variable mismatches as in SAS. The SQLite engine (single file or `:memory:`) and the DB write path are CGo-only, registered when the project's required CGo build is on. **PROC SQL explicit pass-through** sends a database its own native SQL: `connect to <engine> (connection="…")` (or reuse of an assigned libref), `select … from connection to <engine> (<native query>)` returning the result set as a dataset, `execute (<native sql>) by <engine>` for remote DDL/DML, and `drop table <libref>.<member>` routed to the external table — via `table.SQLBackend`/`table.DropBackend`, supported on every database engine. **Implicit query pushdown** sends a value-safe subset of dataset options to the database so it returns less data: `keep=` becomes a column projection, and a `where=` of numeric comparisons using `=`/`>`/`>=` (which exclude missing in SAS exactly as SQL excludes NULL) is pushed as a SQL `WHERE` — everything else (`<`/`<=`/`ne`, which keep missing in SAS; string/function predicates; `drop=`) is filtered locally, so results are always identical to a full read (`table.FilterBackend` → `dbio.Backend.LoadFiltered`). Not yet: pushing joins/aggregation, `obs=`/`firstobs=`, DB2 (needs the CGo IBM CLI driver), and the long tail of SAS/ACCESS options. See [`databases.md`](databases.md).
- JSON harness output (machine-readable report); SAS-byte-identical listing comparison (a non-goal — see "What compatibility means" above; value comparison via `expected.datasets` is the supported mechanism)

See [`../corpus/FEATURES.md`](../corpus/FEATURES.md) for the full feature-tag catalog and intended levels.

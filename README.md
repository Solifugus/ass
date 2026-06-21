# ASS — Analyst's Statistical Suite

<p align="center">
  <img src="mascot.png" alt="The ASS mascot: a scholarly donkey in spectacles, bow tie, and tweed jacket presenting a chart" width="320"><br>
  <em>Analyze. Model. Understand. — open source, evidence-based, built for analysts.</em>
</p>

ASS is an open-source, SAS-compatible data processing and analytics engine written in Go and driven from the command line. It aims for **behavioral compatibility** with a practical subset of SAS programs — the DATA step, PROC PRINT/SORT/SQL, formats, and macro basics — prioritizing real-world ETL and reporting over advanced statistics.

Documentation lives in [`docs/`](docs/). Start here:

- [`tutorial.md`](docs/tutorial.md) — hands-on introduction, hello-world DATA step through SQL, macros, and files.
- [`reference.md`](docs/reference.md) — the complete implemented language surface (statements, functions, formats, PROCs, CLI).
- [`cookbook.md`](docs/cookbook.md) — task-oriented recipes (ETL, joins, reshaping, file round-trips, user formats, stats).

Plus: [`design.md`](docs/design.md) (design rationale), [`PLAN.md`](docs/PLAN.md) (development log), [`COMPATIBILITY.md`](docs/COMPATIBILITY.md) (compatibility matrix and what "compatible" means here), [`databases.md`](docs/databases.md) (external-database LIBNAME engines), and [`CONTRIBUTING.md`](docs/CONTRIBUTING.md) (how to extend).

## Status

Working engine. The lexer → macro → parser → runtime pipeline runs real SAS programs end to end; the bundled compatibility corpus passes **100%** (see `ass test`). Built one tested, corpus-backed feature at a time along the compatibility levels in the design doc — through the advanced DATA step (informats, dataset options, merge/BY-groups, arrays, retain), PROC PRINT/SORT/SQL/MEANS/FREQ (one-/two-way + n-way list, chi-square)/REG/GLM (incl. CLASS)/FORMAT (VALUE + INVALUE), and macro basics. Compatibility is measured at the level of **values/results**, not byte-identical SAS presentation — see [`docs/COMPATIBILITY.md`](docs/COMPATIBILITY.md).

## Building

ASS embeds SQLite for PROC SQL, so **CGo is required**: build with `CGO_ENABLED=1` (the default) and a C compiler (e.g. gcc) installed.

```bash
go build -o ass ./cmd/ass
go test ./...
```

## Usage

```bash
ass file.sas            # run a SAS program (log to stderr, output to stdout)
ass run file.sas        # same, explicit form
ass parse file.sas      # print the parsed AST
ass tokens file.sas     # dump the lexer token stream
ass test corpus/        # run the compatibility corpus and report
ass test --parse-only corpus/
ass test --feature proc-sql corpus/
ass test -v corpus/     # show failure detail
ass test --json corpus/ # machine-readable JSON report (CI/tooling)
```

### Example

```sas
data people;
  input name $ age;
  datalines;
John 25
Mary 30
Tim 12
;
run;

proc sort data=people out=sorted;
  by descending age;
run;

proc print data=sorted;
run;
```

```
$ ass example.sas
NOTE: The data set WORK.PEOPLE has 3 observations and 2 variables.
NOTE: The data set WORK.SORTED has 3 observations and 2 variables.
Obs  name  age

  1  Mary   30
  2  John   25
  3  Tim    12
```

## Supported features

| Area | Highlights |
|------|------------|
| DATA step | `input`/`datalines` (incl. trailing `@`/`@@` line-hold), `infile` (external flat files), `file`/`put` (write flat files, incl. trailing `@`/`@@` output hold, `put _all_`, named `var=` output), `set`, `merge`/`in=`, assignment, `if/then/else`, subsetting `if`, `where`, `do` loops, `retain`, sum statement, arrays, BY-group `first.`/`last.`, `keep`/`drop`, `format`, `label`, `output`, `data _null_` |
| Flat-file input | `infile "path"` with `dlm=`/`delimiter=`, `dsd` (CSV: quoted fields, embedded delimiters, missing), `firstobs=`, `obs=`; list, column (`1-10`), formatted, `@n`/`+n` pointer, and `#n` multi-line input |
| Flat-file output | `file "path"` with `dlm=`/`dsd` (CSV: quotes values containing the delimiter); `put` of variables, string literals, formatted values, column/pointer placement (`name $ 1-10`, `@n`/`+n`), `#n` multi-line output, trailing `@`/`@@` output hold, `put _all_`, and named `var=` output |
| PROC IMPORT/EXPORT | CSV/TAB/DLM delimited files **and `.xlsx` workbooks**: `dbms=csv/tab/dlm/xlsx`, `getnames=`, `datarow=`, `putnames=`, `delimiter=`/`dlm=`; IMPORT sniffs column types, EXPORT writes a header row (the `.xlsx` reader/writer is dependency-free) |
| Dataset options | `(keep= drop= rename=(o=n) where=(...) firstobs= obs=)` on `set`/`merge`/`data`/proc `data=`; numbered var-list ranges (`keep=x1-x5`) |
| Native SAS datasets | `libname lib "/dir";` then read `lib.member` from `member.sas7bdat` — clean-room `.sas7bdat` reader (32/64-bit little-endian; RLE/RDC row compression and uncompressed; numeric, character, dates, formats, labels) |
| Databases (LIBNAME) | `libname pg postgres "…";` then read `pg.table` as a dataset and write it back with `data pg.out; set …;`, `proc sort out=pg.x`, `proc sql; create table pg.x as …;`, or `proc append base=pg.x data=…;` (in-place INSERT) — Postgres, SQL Server, Oracle, SQLite, and DB2 (DB2 via `-tags db2`); see [`docs/databases.md`](docs/databases.md) |
| Expressions | arithmetic, comparison, logical, concatenation, ~35 functions, SAS missing-value & type-coercion semantics |
| PROC PRINT | `var`, `noobs`, `label` (renders variable labels, incl. a step `label` statement), applied formats |
| PROC SORT | `by` (+ `descending`), `out=` (incl. a database libref), `nodupkey` |
| PROC APPEND | `base=`/`data=` (+ `force`): append observations to a base data set, created if absent; BASE= or DATA= may be a database libref (in-place INSERT) |
| PROC SQL | `select`/`where`/`order by`/joins/`group by`, `create table as` (WORK or a database libref; via embedded SQLite) |
| PROC MEANS/SUMMARY | N, Mean, StdDev, Min, Max with `class`/`by` (CLASS groups by user formats) |
| PROC FREQ | one-way frequency tables, two-way cross-tabulation (`tables a*b`), n-way list tables (`/ list`), `/ options` (nocol/norow/nopercent/nofreq/nocum), and `/ chisq` (Pearson chi-square); groups by user formats |
| PROC REG/GLM | OLS linear regression: estimates, std err, t-value, `Pr>|t|`, R²; CLASS categorical predictors (reference-cell coding) |
| PROC FORMAT | user-defined `value` formats (ranges, `low`/`high`, `other`, char), applied in PROC PRINT and for grouping in PROC FREQ/MEANS; `invalue` user informats read by INPUT |
| Macros | `%let`/`&var`, `%macro`/`%mend` (positional + keyword params), `%do`, `%if/%then/%else` |
| Formats | `w.d`, `dollar`, `comma`, `percent`, `$w.`, date (`date9`/`mmddyy`/`worddate`), `time`/`datetime`, date/time/datetime literals `'01JAN2020'd`/`'14:30:00't`/`'01JAN2020:14:30:00'dt` |
| Informats | list input via `:` modifier: `comma`, `dollar`, `date9`, `mmddyy`/`ddmmyy`/`yymmdd`, `time`/`datetime`, `$w.` |

Not yet supported (selected): big-endian `.sas7bdat` files (32/64-bit little-endian — uncompressed and RLE/RDC row-compressed — are read; delimited CSV/TAB/DLM, `.xlsx` import/export, and native `.sas7bdat` read are supported), the default stratified PROC FREQ n-way layout (`/ list` n-way and `/ chisq` are supported) and association statistics beyond Pearson chi-square, and SAS GLM's generalized-inverse parameterization / Type I-III SS / LSMEANS (CLASS effects work via reference-cell coding, which differs from SAS's per-level estimates by convention). See [`docs/COMPATIBILITY.md`](docs/COMPATIBILITY.md) and [`corpus/FEATURES.md`](corpus/FEATURES.md).

## Contributing

See [`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md) for how to add a corpus item, run the harness, and implement a new PROC or function.

## Disclaimer

Analyst's Statistical Suite is an independent open-source project. It is not affiliated with, endorsed by, or sponsored by SAS Institute Inc. "SAS" is a trademark of SAS Institute Inc. ASS implements behavioral compatibility through clean-room methods based on public examples.

## License

MIT — see [`LICENSE`](LICENSE).

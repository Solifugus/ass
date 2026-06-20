# ASS — Analyst's Statistical Suite

<p align="center">
  <img src="mascot.png" alt="The ASS mascot: a scholarly donkey in spectacles, bow tie, and tweed jacket presenting a chart" width="320"><br>
  <em>Analyze. Model. Understand. — open source, evidence-based, built for analysts.</em>
</p>

ASS is an open-source, SAS-compatible data processing and analytics engine written in Go and driven from the command line. It aims for **behavioral compatibility** with a practical subset of SAS programs — the DATA step, PROC PRINT/SORT/SQL, formats, and macro basics — prioritizing real-world ETL and reporting over advanced statistics.

Documentation lives in [`docs/`](docs/): [`design.md`](docs/design.md) (design rationale), [`PLAN.md`](docs/PLAN.md) (development log), [`COMPATIBILITY.md`](docs/COMPATIBILITY.md) (compatibility matrix and what "compatible" means here), and [`CONTRIBUTING.md`](docs/CONTRIBUTING.md) (how to extend).

## Status

Working engine. The lexer → macro → parser → runtime pipeline runs real SAS programs end to end; the bundled compatibility corpus passes **100%** (see `ass test`). Built one tested, corpus-backed feature at a time along the compatibility levels in the design doc — through the advanced DATA step (informats, dataset options, merge/BY-groups, arrays, retain), PROC PRINT/SORT/SQL/MEANS/FREQ (one- and two-way)/REG/GLM (incl. CLASS)/FORMAT, and macro basics. Compatibility is measured at the level of **values/results**, not byte-identical SAS presentation — see [`docs/COMPATIBILITY.md`](docs/COMPATIBILITY.md).

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
| DATA step | `input`/`datalines`, `infile` (external flat files), `file`/`put` (write flat files), `set`, `merge`/`in=`, assignment, `if/then/else`, subsetting `if`, `where`, `do` loops, `retain`, sum statement, arrays, BY-group `first.`/`last.`, `keep`/`drop`, `format`, `label`, `output`, `data _null_` |
| Flat-file input | `infile "path"` with `dlm=`/`delimiter=`, `dsd` (CSV: quoted fields, embedded delimiters, missing), `firstobs=`, `obs=`; list, column (`1-10`), formatted, and `@n`/`+n` pointer input |
| Flat-file output | `file "path"` with `dlm=`/`dsd` (CSV: quotes values containing the delimiter); `put` of variables, string literals, formatted values, and column/pointer placement (`name $ 1-10`, `@n`/`+n`) |
| PROC IMPORT/EXPORT | CSV/TAB/DLM delimited files: `dbms=csv/tab/dlm`, `getnames=`, `datarow=`, `putnames=`, `delimiter=`/`dlm=`; IMPORT sniffs column types, EXPORT writes a header row |
| Dataset options | `(keep= drop= rename=(o=n) where=(...))` on `set`/`merge`/`data`/proc `data=` |
| Native SAS datasets | `libname lib "/dir";` then read `lib.member` from `member.sas7bdat` — clean-room `.sas7bdat` reader (32/64-bit little-endian; RLE/RDC row compression and uncompressed; numeric, character, dates, formats, labels) |
| Databases (LIBNAME) | `libname pg postgres "…";` then read `pg.table` as a dataset and write it back with `data pg.out; set …;`, `proc sort out=pg.x`, or `proc sql; create table pg.x as …;` — Postgres, SQL Server, Oracle, SQLite; see [`docs/databases.md`](docs/databases.md) |
| Expressions | arithmetic, comparison, logical, concatenation, ~35 functions, SAS missing-value & type-coercion semantics |
| PROC PRINT | `var`, `noobs`, `label` (renders variable labels, incl. a step `label` statement), applied formats |
| PROC SORT | `by` (+ `descending`), `out=` (incl. a database libref), `nodupkey` |
| PROC SQL | `select`/`where`/`order by`/joins/`group by`, `create table as` (WORK or a database libref; via embedded SQLite) |
| PROC MEANS/SUMMARY | N, Mean, StdDev, Min, Max with `class`/`by` |
| PROC FREQ | one-way frequency tables and two-way cross-tabulation (`tables a*b`) |
| PROC REG/GLM | OLS linear regression: estimates, std err, t-value, `Pr>|t|`, R²; CLASS categorical predictors (reference-cell coding) |
| PROC FORMAT | user-defined `value` formats (ranges, `low`/`high`, `other`, char), applied in PROC PRINT |
| Macros | `%let`/`&var`, `%macro`/`%mend` (positional + keyword params), `%do`, `%if/%then/%else` |
| Formats | `w.d`, `dollar`, `comma`, `percent`, `$w.`, date (`date9`/`mmddyy`/`worddate`), date literals `'01JAN2020'd` |
| Informats | list input via `:` modifier: `comma`, `dollar`, `date9`, `mmddyy`/`ddmmyy`/`yymmdd`, `$w.` |

Not yet supported (selected): multi-line input/output (`#n` line pointers, trailing `@`/`@@` line-hold; single-line column/pointer input and output `@`/`+n`/`1-10` ranges are supported), big-endian `.sas7bdat` files (32/64-bit little-endian — uncompressed and RLE/RDC row-compressed — are read; `.xlsx` import/export — delimited CSV/TAB/DLM and native `.sas7bdat` read are supported), PROC FREQ n-way tables and association statistics, and SAS GLM's generalized-inverse parameterization / Type I-III SS / LSMEANS (CLASS effects work via reference-cell coding, which differs from SAS's per-level estimates by convention). See [`docs/COMPATIBILITY.md`](docs/COMPATIBILITY.md) and [`corpus/FEATURES.md`](corpus/FEATURES.md).

## Contributing

See [`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md) for how to add a corpus item, run the harness, and implement a new PROC or function.

## Disclaimer

Analyst's Statistical Suite is an independent open-source project. It is not affiliated with, endorsed by, or sponsored by SAS Institute Inc. "SAS" is a trademark of SAS Institute Inc. ASS implements behavioral compatibility through clean-room methods based on public examples.

## License

MIT — see [`LICENSE`](LICENSE).

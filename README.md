# ASS — Analyst's Statistical Suite

ASS is an open-source, SAS-compatible data processing and analytics engine written in Go and driven from the command line. It aims for **behavioral compatibility** with a practical subset of SAS programs — the DATA step, PROC PRINT/SORT/SQL, formats, and macro basics — prioritizing real-world ETL and reporting over advanced statistics.

See [`ass-design.md`](ass-design.md) for the design rationale, [`PLAN.md`](PLAN.md) for the development log, and [`COMPATIBILITY.md`](COMPATIBILITY.md) for the current compatibility matrix.

## Status

Working engine. The lexer → macro → parser → runtime pipeline runs real SAS programs end to end; the bundled compatibility corpus passes **100%** (see `ass test`). Built one tested, corpus-backed feature at a time along the compatibility levels in the design doc — currently through **Level 5 (advanced DATA step) plus PROC SQL, macros, and basic statistical procedures**.

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
| DATA step | `input`/`datalines`, `set`, `merge`/`in=`, assignment, `if/then/else`, subsetting `if`, `where`, `do` loops, `retain`, sum statement, arrays, BY-group `first.`/`last.`, `keep`/`drop`, `format`, `output` |
| Dataset options | `(keep= drop= rename=(o=n) where=(...))` on `set`/`merge`/`data`/proc `data=` |
| Expressions | arithmetic, comparison, logical, concatenation, ~35 functions, SAS missing-value & type-coercion semantics |
| PROC PRINT | `var`, `noobs`, `label`, applied formats |
| PROC SORT | `by` (+ `descending`), `out=`, `nodupkey` |
| PROC SQL | `select`/`where`/`order by`/joins/`group by`, `create table as` (via embedded SQLite) |
| PROC MEANS/SUMMARY | N, Mean, StdDev, Min, Max with `class`/`by` |
| PROC FREQ | one-way frequency tables and two-way cross-tabulation (`tables a*b`) |
| PROC REG/GLM | OLS linear regression: estimates, std err, t-value, `Pr>|t|`, R²; CLASS categorical predictors (reference-cell coding) |
| PROC FORMAT | user-defined `value` formats (ranges, `low`/`high`, `other`, char), applied in PROC PRINT |
| Macros | `%let`/`&var`, `%macro`/`%mend` (positional + keyword params), `%do`, `%if/%then/%else` |
| Formats | `w.d`, `dollar`, `comma`, `percent`, `$w.`, date (`date9`/`mmddyy`/`worddate`), date literals `'01JAN2020'd` |
| Informats | list input via `:` modifier: `comma`, `dollar`, `date9`, `mmddyy`/`ddmmyy`/`yymmdd`, `$w.` |

Not yet supported (selected): column/pointer input, PROC FREQ n-way tables and association statistics, and SAS GLM's generalized-inverse parameterization / Type I-III SS / LSMEANS (CLASS effects work via reference-cell coding, which differs from SAS's per-level estimates by convention). See [`COMPATIBILITY.md`](COMPATIBILITY.md) and `corpus/FEATURES.md`.

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for how to add a corpus item, run the harness, and implement a new PROC or function.

## Disclaimer

Analyst's Statistical Suite is an independent open-source project. It is not affiliated with, endorsed by, or sponsored by SAS Institute Inc. "SAS" is a trademark of SAS Institute Inc. ASS implements behavioral compatibility through clean-room methods based on public examples.

## License

MIT — see [`LICENSE`](LICENSE).

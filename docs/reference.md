# ASS Reference

The implemented surface of **ASS — Analyst's Statistical Suite**, organized for
lookup. This documents *what ASS actually does today*; intentionally-deferred SAS
features are listed at the end and in [`COMPATIBILITY.md`](COMPATIBILITY.md).

New to ASS? Start with the [**Tutorial**](tutorial.md). For task-oriented
examples, see the [**Cookbook**](cookbook.md).

- [Program structure](#program-structure)
- [DATA step statements](#data-step-statements)
- [INPUT / reading data](#input--reading-data)
- [PUT / FILE — writing flat files](#put--file--writing-flat-files)
- [Dataset options](#dataset-options)
- [Expressions & operators](#expressions--operators)
- [Functions](#functions)
- [Formats](#formats)
- [Informats](#informats)
- [Date/time literals](#datetime-literals)
- [PROCs](#procs)
- [LIBNAME engines](#libname-engines)
- [Macro language](#macro-language)
- [CLI](#cli)
- [Not implemented](#not-implemented)

---

## Program structure

A program is a sequence of **steps**, each terminated by `run;` (or `quit;` for
PROC SQL). Steps are parsed and executed one at a time and share data only
through named datasets.

- **DATA step** — `data NAME; ... run;` builds a dataset by running its body once
  per input row (the *implicit loop*) over the Program Data Vector (PDV).
- **PROC step** — `proc NAME data=...; ... run;` consumes datasets to produce
  reports, statistics, or new datasets.

Automatic variables in a DATA step: `_N_` (iteration counter) and `_ERROR_`.
The default library is `WORK`; an unqualified dataset name `x` means `WORK.x`.

Comments: `/* ... */` and `* ... ;`.

---

## DATA step statements

| Statement | Purpose |
|-----------|---------|
| `data NAME ... ;` | Open a step; one or more output datasets. `data _null_;` produces none. |
| `input ...;` | Read fields from `datalines`/`infile` into the PDV. See [INPUT](#input--reading-data). |
| `datalines;` / `cards;` | Begin inline data; the block ends at a line containing only `;`. |
| `infile "path" <opts>;` | Read an external flat file. Options: `dlm=`/`delimiter=`, `dsd`, `firstobs=`, `obs=`. |
| `set DS <(opts)>;` | Read rows from an existing dataset. |
| `merge DS1<(in=a)> DS2<(in=b)>;` | Match-merge by the following `by`; `in=` flags row provenance. |
| `by VAR <descending VAR> ...;` | Define BY groups; activates `first.VAR`/`last.VAR`. |
| `VAR = expr;` | Assignment. Variable type is set by first use. |
| `if cond then stmt; else stmt;` | Conditional execution. |
| `if cond;` | **Subsetting if** — drop rows where `cond` is false. |
| `where expr;` | Row filter applied as data is read. |
| `do ...; end;` | Iterative/`do i = a to b <by c>` and plain `do` blocks. |
| `retain VAR <init> ...;` | Preserve values across iterations (optionally initialize). |
| `VAR + expr;` | **Sum statement** — accumulate (auto-retain, treat missing as 0). |
| `array A{n} v1 ... ;` | Define an array over variables; index `A{i}`. |
| `output <DS>;` | Write the current PDV as a row (to `DS` if named). |
| `keep VAR ...;` / `drop VAR ...;` | Restrict output variables (range form `x1-x5` allowed). |
| `rename old=new ...;` | Rename variables in the output dataset(s). Within the step the original names are used; the rename applies when the output is written (the `rename=` dataset option is the other form). |
| `format VAR fmt. ...;` | Attach output formats. |
| `label VAR="text" ...;` | Attach variable labels. |
| `file "path" <opts>;` | Direct `put` output to a file. Options: `dlm=`, `dsd`. |
| `put ...;` | Write to the current `file` (or the log). See [PUT](#put--file--writing-flat-files). |

When no explicit `output` appears, the step writes one row at the bottom of each
iteration.

---

## INPUT / reading data

`input` supports the SAS input modes, mixable in one statement:

| Mode | Syntax | Notes |
|------|--------|-------|
| List | `input name $ age;` | Whitespace-delimited, in order. `$` ⇒ character. |
| List + informat | `input pay : comma8.;` | `:` applies an informat to a delimited field. |
| Column | `input name $ 1-10 age 11-13;` | Fixed columns. |
| Formatted | `input @1 name $10. @11 age 3.;` | Informat-driven at a column. |
| Pointer `@n` | `input @5 code $4.;` | Absolute column. |
| Pointer `+n` | `input id +2 name $8.;` | Relative skip. |
| Line `#n` | `input #1 a #2 b;` | Multi-line records. |
| Trailing `@` | `input id @;` | Hold the line for a later `input`. |
| Trailing `@@` | `input x @@;` | Hold across iterations (multiple records per line). |

`infile` options: `dlm=`/`delimiter="c"` (field separator), `dsd` (CSV rules —
quoted fields, embedded delimiters, two delimiters ⇒ missing), `firstobs=n`
(start line), `obs=n` (last line).

---

## PUT / FILE — writing flat files

`file "path"` selects the output file (options `dlm=`, `dsd`); `put` writes to
it. `data _null_;` is the usual host for file-writing steps.

| `put` form | Effect |
|------------|--------|
| `put a b c;` | List output of variables (separated by spaces, or `dlm=`/`dsd`). |
| `put "literal" x;` | Mix string literals and values. |
| `put x dollar10.2;` | Formatted output. |
| `put name $ 1-10 age 11-13;` | Column placement. |
| `put @5 code +2 x;` | Pointer placement (`@n`, `+n`). |
| `put #2 b;` | Multi-line output. |
| `put x @;` / `put x @@;` | Hold the output line (don't end it yet). |
| `put _all_;` | Dump every PDV variable as `name=value`. |
| `put name=;` | Named output: emit `name=value`. |

---

## Dataset options

Written in parentheses after a dataset reference on `set`, `merge`, `data`, or a
PROC's `data=`:

| Option | Meaning |
|--------|---------|
| `keep=v1 v2` | Keep only these variables (range `x1-x5` allowed). |
| `drop=v1 v2` | Drop these variables (range allowed). |
| `rename=(old=new ...)` | Rename variables. |
| `where=(expr)` | Filter rows. |
| `in=name` | (on `merge`/`set`) Boolean: did this source contribute the row? |
| `firstobs=n` | Start reading at row *n*. |
| `obs=n` | Stop after row *n*. |

Example: `set big(keep=id amt where=(amt>0) firstobs=10 obs=100);`

---

## Expressions & operators

- **Arithmetic:** `+ - * / **` (`**` is power).
- **Comparison:** `= ^= < <= > >=` (and word forms `eq ne lt le gt ge`).
- **Logical:** `and or not` (and `&`, `|`).
- **Concatenation:** `||`.
- **Missing values:** numeric missing is `.`; it sorts low and propagates through
  arithmetic per SAS rules. Functions follow SAS missing-aware semantics.
- **Type coercion:** numeric↔character coercion follows SAS conventions in mixed
  expressions.

---

## Functions

All implemented functions (49). Aggregates ignore missing values and accept a
variable number of arguments.

**Aggregate / numeric reductions**

| Function | Result |
|----------|--------|
| `sum(...)` | Sum of non-missing arguments. |
| `mean(...)` / `avg(...)` | Mean of non-missing arguments. |
| `min(...)` / `max(...)` | Extremes of non-missing arguments. |
| `n(...)` | Count of non-missing numeric arguments. |
| `nmiss(...)` | Count of missing arguments. |

**Math**

| Function | Result |
|----------|--------|
| `abs(x)` | Absolute value. |
| `int(x)` | Truncate toward zero. |
| `ceil(x)` / `floor(x)` | Round up / down to integer. |
| `round(x, u)` | Round `x` to nearest multiple of `u` (default 1). |
| `sqrt(x)` | Square root. |
| `exp(x)` / `log(x)` | e^x / natural log. |

**Character**

| Function | Result |
|----------|--------|
| `upcase(s)` / `lowcase(s)` | Change case. |
| `propcase(s)` | Capitalize each word. |
| `trim(s)` | Remove trailing blanks. |
| `strip(s)` | Remove leading and trailing blanks. |
| `left(s)` | Left-align (drop leading blanks). |
| `length(s)` | Length excluding trailing blanks. |
| `substr(s, pos, len)` | Substring (1-based; `len` optional). |
| `cats(...)` | Concatenate, stripping each argument. |
| `catx(sep, ...)` | Concatenate with separator, stripping each argument. |
| `index(s, sub)` | 1-based position of `sub` in `s` (0 if absent). |
| `find(s, sub)` | Position of `sub` in `s` (0 if absent). |
| `scan(s, n <,delims>)` | The *n*-th word of `s`. |
| `compress(s <,chars>)` | Remove characters (blanks by default, or those in `chars`). |
| `tranwrd(s, from, to)` | Replace all occurrences of `from` with `to`. |
| `reverse(s)` | Reverse the string. |

**Missing test**

| Function | Result |
|----------|--------|
| `missing(x)` | 1 if `x` is missing, else 0. |

**Date / time** — a SAS date is days since 1960-01-01, a datetime is seconds since
1960-01-01 00:00:00, a time is seconds since midnight. Missing arguments
propagate.

| Function | Result |
|----------|--------|
| `today()` / `date()` | Current date as a SAS day number. |
| `datetime()` | Current datetime value. |
| `time()` | Current time of day (seconds since midnight). |
| `mdy(m, d, y)` | SAS date for that calendar date (missing if invalid, e.g. 30FEB). |
| `year(d)` / `month(d)` / `day(d)` | Calendar parts of a SAS date. |
| `qtr(d)` | Quarter (1–4). |
| `weekday(d)` | Day of week, Sunday=1 … Saturday=7. |
| `datepart(dt)` / `timepart(dt)` | Date / time part of a SAS datetime. |
| `hms(h, m, s)` | Time value from hours/minutes/seconds. |
| `dhms(d, h, m, s)` | Datetime value from a date plus h/m/s. |
| `intck(interval, from, to)` | Count of interval boundaries (`day`, `week`, `month`, `qtr`, `semiyear`, `year`, `hour`, `minute`, `second`). |
| `intnx(interval, start, n <,align>)` | Advance `start` by `n` intervals; align `b`(egin, default)/`m`(iddle)/`e`(nd)/`s`(ame). |

Both accept a multiplier and shift on the interval name (`month2` = bimonthly, `week.2` = weeks starting Monday, `qtr2.2`), and a `dt` prefix to apply a calendar interval to datetime values (`dtday`, `dtmonth`, `dtqtr`, `dtyear`).

**Format / informat**

| Function | Result |
|----------|--------|
| `put(value, format.)` | The value rendered through a format (user VALUE format or built-in), returned as a character string — e.g. `put(score, scoreband.)` bands a number, `put(x, dollar10.2)` → `$1,234.00`. |
| `input(string, informat.)` | A character string read through an informat (user INVALUE or built-in), returning the value — e.g. `input("1,234", comma8.)` → 1234, `input("15JAN2020", date9.)` → a SAS date. |

---

## Formats

Output formats for the `format` statement, `put`, and PROC display. Width/decimal
follow the SAS `w.d` convention.

| Format | Example | Renders |
|--------|---------|---------|
| `w.d` | `8.2` | Fixed numeric, `d` decimals. |
| `dollar w.d` | `dollar10.2` | `$1,234.00` |
| `comma w.d` | `comma10.0` | `1,234` |
| `percent w.d` | `percent8.1` | `12.3%` |
| `$w.` | `$10.` | Character, width `w`. |
| `date9.` | | `15JAN2020` |
| `mmddyy w.` | `mmddyy10.` | `01/15/2020` |
| `worddate.` | | `January 15, 2020` |
| `time w.` | `time8.` | `14:30:00` |
| `datetime w.` | `datetime19.` | `15JAN2020:14:30:00` |

User-defined formats come from `PROC FORMAT VALUE` (see [PROC FORMAT](#proc-format)).

---

## Informats

Read with the `:` modifier in list input (`var : informat.`) or as formatted
input.

| Informat | Reads |
|----------|-------|
| `comma w.` | Number with thousands separators (`1,234` ⇒ 1234). |
| `dollar w.` | Currency text ⇒ number. |
| `$w.` | Character, width `w`. |
| `date9.` | `15JAN2020` ⇒ SAS date. |
| `mmddyy w.` / `ddmmyy w.` / `yymmdd w.` | Numeric date strings ⇒ SAS date. |
| `time w.` | `14:30:00` ⇒ SAS time (seconds). |
| `datetime w.` | `01JAN2020:14:30:00` ⇒ SAS datetime. |

User-defined informats come from `PROC FORMAT INVALUE`.

---

## Date/time literals

| Literal | Value |
|---------|-------|
| `'01JAN2020'd` | SAS date — days since 1960-01-01. |
| `'14:30:00't` | SAS time — seconds since midnight. |
| `'01JAN2020:14:30:00'dt` | SAS datetime — seconds since 1960-01-01 00:00:00. |

---

## PROCs

Registered procedures: `print`, `sort`, `means`, `summary`, `freq`, `sql`,
`reg`, `glm`, `format`, `import`, `export`, `append`.

### PROC PRINT

```sas
proc print data=DS noobs label;
  var v1 v2 ...;
run;
```
- `noobs` — suppress the `Obs` column.
- `label` — show variable labels as headers.
- `var` — choose and order columns.
- Applied formats (from `format` statements) are honored.

### PROC SORT

```sas
proc sort data=DS out=OUT nodupkey;
  by v1 descending v2;
run;
```
- `out=` — destination (in place if omitted); may be a database libref.
- `descending` — per-key reverse order.
- `nodupkey` — drop rows duplicating the BY key.

### PROC MEANS / SUMMARY

```sas
proc means data=DS n mean stddev min max sum maxdec=2;
  class g;        /* or: by g; */
  var x y;
run;
```
Statistics: `n`, `mean`, `stddev` (`std`), `min`, `max`, `sum`. The keywords you
list select which statistics appear and in what order; with none given, the SAS
default set `N Mean StdDev Min Max` is used. `maxdec=k` fixes the displayed
decimal places of the statistic columns (the `N` count stays an integer).
`class`/`by` group the output; CLASS groups respect user formats.

### PROC FREQ

```sas
proc freq data=DS;
  tables a;                 /* one-way */
  tables a*b;               /* two-way cross-tab */
  tables a*b*c / list;      /* n-way list layout */
  tables a*b / chisq nocol norow nopercent nocum nofreq;
run;
```
- One-way, two-way cross-tabulation, and n-way `/ list` tables.
- `/ chisq` — Pearson chi-square (with p-value).
- Display options: `nofreq`, `nopercent`, `nocum`, `norow`, `nocol`. On one-way
  and `/ list` layouts these drop the matching column; on the two-way cross-tab
  `nofreq`/`nopercent`/`norow`/`nocol` drop the matching cell statistic
  (frequency / cell percent / row percent / column percent). Suppressing all four
  falls back to showing the frequency.
- Counts respect user formats for grouping.

### PROC SQL

```sas
proc sql;
  create table OUT as
    select a, sum(b) as t from DS where a>0 group by a having t>10 order by t desc;
quit;
```
- `select`/`where`/`group by`/`having`/`order by`/joins; `create table ... as`.
- Backed by embedded SQLite via the **pure-Go** `modernc.org/sqlite` driver, so
  PROC SQL builds and runs with `CGO_ENABLED=0` (no C compiler) like the rest of
  the engine; ends with `quit;`. See the README's build matrix.
- Sources may be WORK datasets or a database libref; `create table` may target a
  database libref.
- A result column carried through from a source column (same name) inherits that
  column's format, so a bare `select` listing renders user/built-in formats as
  PROC PRINT would; aggregates and renamed columns are unformatted.
- **Pass-through:** `select ... from connection to LIB (native-sql)` runs SQL on
  the bound database server and returns the result. See
  [`databases.md`](databases.md).

### PROC REG / GLM

```sas
proc reg data=DS;
  model y = x1 x2;
run;

proc glm data=DS;
  class grp;
  model y = grp x;
run;
```
OLS linear regression: parameter estimates, standard error, t-value, `Pr>|t|`,
R². `PROC GLM` adds `class` categorical predictors via **reference-cell coding**.
(SAS's generalized-inverse parameterization, Type I/III SS, and LSMEANS are
deferred — see [below](#not-implemented).)

### PROC FORMAT

```sas
proc format;
  value agegrp low-17="minor" 18-high="adult" other="?";
  value $reg "N"="North" "S"="South";
  invalue yn "Y"=1 "N"=0;
  picture dollars low-high='000,000,009.99' (prefix='$');
run;
```
- `value` — output formats: numeric ranges, `low`/`high`, `other`, and `$`
  character formats. Applied in PROC PRINT and for grouping in FREQ/MEANS.
- `invalue` — user informats read by `input`.
- `picture` — output picture templates: digit selectors (`0`-`9`; a `0` selector
  zero-suppresses leading positions, a nonzero selector forces printing) with
  literal message characters, scaled by a default or `mult=` multiplier; options
  `prefix=`, `mult=`, `fill=`. Applied in PROC PRINT and via `put()`.

### PROC PROOF

Data-quality validation — an ASS value-add (not a SAS procedure). Checks a
dataset against declared assertions and emits a verdict: a report, an optional
`out=` violations dataset, and a non-zero process exit when an error-level
assertion fails. See [`proofing.md`](proofing.md).

```sas
proc proof data=orders out=bad maxsample=20 severity=error;
  require id qty;                          /* columns must exist               */
  type id=num region=char;                /* declared kinds must match         */
  notnull qty / severity=warn;            /* values present (warn, not error)  */
  values region in ("east" "west");        /* domain / allowed set             */
  range qty 1 - 100;                       /* inclusive numeric bound          */
  unique id;                               /* no duplicate keys                */
  key region references regions(region);   /* referential integrity            */
  rule "ship after order": shipdate >= orderdate;  /* arbitrary boolean       */
run;
```
- Assertions: `require`, `type <var>=num|char`, `notnull`, `values … in (…)`,
  `range <var> lo - hi` or the relational `range <var> >=|<=|>|<|=|^= <num>`,
  `unique <vars>`, `key <cols> references <parent>(<cols>)` (single- or
  multi-column), `rule "label": <expr>`.
- Each assertion may carry `/ severity=warn|error message="…"` — including `rule`
  (whose expression captures `/` as division up to the tail).
- `out=` receives one row per (source row × failed assertion), annotated with
  `_rule_` and `_obs_`, so violations are trivially filterable.
- Error-level failures log `ERROR` and make the CLI exit non-zero **without**
  halting the program; warn-level failures log `WARNING` and don't affect the exit
  code.
- A foreign key with any missing component passes (`key`), mirroring SQL NULL-FK
  semantics; the parent is resolved through the library (WORK, base, or database
  libref).
- Deferred: `abort` (fail-fast) and the statistical tier (see proofing.md §11).

### PROC IMPORT / EXPORT

```sas
proc import datafile="in.csv"  out=work.d dbms=csv  replace;
  getnames=yes; datarow=2;
run;

proc export data=work.d outfile="out.xlsx" dbms=xlsx replace;
run;
```
- `dbms=csv|tab|dlm|xlsx`. `delimiter=`/`dlm=` for `dlm=`.
- IMPORT: `getnames=` (header row), `datarow=`, sniffs column types.
- EXPORT: writes a header row; `putnames=`.
- The `.xlsx` reader/writer is dependency-free (single worksheet, header + data).

### PROC APPEND

```sas
proc append base=master data=new force;
run;
```
Append observations of `data=` onto `base=` (created if absent). `force`
reconciles variable mismatches. Either side may be a database libref (in-place
INSERT).

---

## LIBNAME engines

```sas
libname lib "/dir";                       /* base engine: native .sas7bdat */
libname pg postgres "host=... dbname=...";/* database engine */
```

- **Base/directory:** `lib.member` reads `dir/member.sas7bdat` — clean-room
  reader for 32/64-bit little-endian files, uncompressed and RLE/RDC
  row-compressed; numeric, character, dates, formats, labels. Big-endian files
  are detected and rejected.
- **Database:** binds a libref to Postgres, SQL Server, Oracle, SQLite, or DB2
  (DB2 needs `-tags db2`). DB tables read as datasets and DATA steps / PROCs can
  write them back; `proc append` does in-place INSERT. Connection strings and
  semantics: [`databases.md`](databases.md).

---

## Macro language

Runs **before** the parser (textual code generation).

| Construct | Purpose |
|-----------|---------|
| `%let name = value;` | Define a macro variable. |
| `&name` / `&name.` | Reference (the dot ends the name). |
| `%macro m(p1, p2=default); ... %mend;` | Define a macro (positional + keyword params). |
| `%m(arg1, p2=x)` | Invoke a macro. |
| `%if cond %then ...; %else ...;` | Conditional generation. |
| `%do i = a %to b; ... %end;` | Iterative generation. |

---

## CLI

```bash
ass file.sas                  # run (NOTE log to stderr, output to stdout)
ass run file.sas              # explicit run
ass parse file.sas            # dump the parsed AST
ass tokens file.sas           # dump the lexer token stream
ass test corpus/              # run the compatibility corpus, report pass rates
ass test --parse-only corpus/ # parse without executing
ass test --feature TAG corpus/# run only items carrying a feature tag
ass test -v corpus/           # show failure detail
ass test --json corpus/       # machine-readable JSON report
ass test --coverage corpus/   # per-feature value-verification backlog (gaps first)
```

---

## Not implemented

Selected intentional deferrals (full list and rationale in
[`COMPATIBILITY.md`](COMPATIBILITY.md)):

- The default stratified PROC FREQ n-way layout (only `/ list` and `/ chisq`),
  and association statistics beyond Pearson chi-square.
- SAS GLM's generalized-inverse parameterization, Type I/III SS, and LSMEANS
  (CLASS effects work via reference-cell coding).
- Big-endian `.sas7bdat` files (little-endian, compressed and not, are read).

# ASS Tutorial

A hands-on, build-up introduction to **ASS — Analyst's Statistical Suite**. Work
through it top to bottom; each section runs a complete program and adds one new
idea. If you know SAS, this will feel familiar — ASS targets *behavioral*
compatibility with a practical subset of the SAS language.

> Companion docs: the [**Reference**](reference.md) lists every implemented
> statement, function, format, and PROC; the [**Cookbook**](cookbook.md) gives
> task-oriented recipes.

## 0. Setup

ASS is a single Go binary. Build it once (CGo is required because PROC SQL embeds
SQLite):

```bash
CGO_ENABLED=1 go build -o ass ./cmd/ass
```

Run a program by handing the file to `ass`:

```bash
ass program.sas          # run it (NOTE: lines + listings to your terminal)
ass run program.sas      # identical, explicit form
ass parse program.sas    # show the parsed AST instead of running
ass tokens program.sas   # dump the lexer's token stream
```

Put each example below in a file (say `t.sas`) and run `ass t.sas`.

## 1. Hello, DATA step

A program is a sequence of **steps**. The two kinds are the **DATA step** (which
builds datasets) and **PROC steps** (which consume them). Every step ends with
`run;`.

```sas
data hello;
  greeting = "Hello, ASS";
  x = 6 * 7;
run;

proc print data=hello;
run;
```

```
NOTE: The data set WORK.HELLO has 1 observations and 2 variables.
Obs  greeting     x
  1  Hello, ASS  42
```

What happened:

- `data hello;` opens a step that writes a dataset named `HELLO` in the default
  library `WORK`.
- Each statement runs once per row. With no input, the step runs its body **one
  time**, builds one row, and writes it automatically at the bottom of the step.
- Variables are typed by first use: `greeting` is character (assigned a string),
  `x` is numeric.

## 2. Reading inline data with `datalines`

Most DATA steps read data. The simplest source is `datalines` (inline rows) read
by an `input` statement:

```sas
data people;
  input name $ age;
  datalines;
John 25
Mary 30
Tim 12
;
run;

proc print data=people;
run;
```

- `input name $ age;` — read two fields per line. The `$` marks `name` as
  **character**; `age` (no `$`) is numeric.
- The block between `datalines;` and the closing `;` is the raw data.
- The step now loops **once per data line**, filling the Program Data Vector
  (PDV) and writing a row each iteration.

This is *list input*: fields are whitespace-delimited and read in order.

## 3. Computing, conditionals, and the implicit loop

Add derived columns and branch with `if/then/else`:

```sas
data classified;
  input name $ age;
  if age >= 18 then status = "adult";
  else status = "minor";
  decade = int(age / 10) * 10;
  datalines;
John 25
Mary 30
Tim 12
;
run;

proc print data=classified;
run;
```

Each row flows through the body top to bottom. `int()` is one of the built-in
functions (see the [Reference](reference.md#functions)). The new variables join
the PDV and are written out with the rest.

### Keeping only some rows — the subsetting `if`

An `if` with no `then` is a **subsetting if**: rows that fail it are dropped.

```sas
data adults;
  input name $ age;
  if age >= 18;        /* keep only adults */
  datalines;
John 25
Mary 30
Tim 12
;
run;
```

`ADULTS` has two rows; `Tim` never reaches `output`.

## 4. Sorting and printing

`PROC SORT` orders a dataset; `PROC PRINT` lists it.

```sas
proc sort data=people out=byage;
  by descending age;
run;

proc print data=byage noobs;
  var name age;
run;
```

- `out=byage` writes the sorted result to a new dataset (omit it to sort in
  place).
- `by descending age` sorts high to low; list several variables for nested sorts.
- `noobs` drops the `Obs` column; `var name age;` chooses and orders columns.

## 5. BY-group processing: `first.` / `last.`

Once data is sorted by a key, the DATA step exposes automatic `first.VAR` and
`last.VAR` flags — the backbone of per-group logic.

```sas
proc sort data=sales out=s; by region; run;

data totals;
  set s;
  by region;
  retain running 0;
  if first.region then running = 0;   /* reset at group start */
  running + amount;                    /* sum statement: running = running + amount */
  if last.region then output;          /* emit one row per group */
  keep region running;
run;
```

Key pieces:

- `set s;` reads an existing dataset row by row (the analog of `input` for
  datasets).
- `by region;` activates `first.region` / `last.region`.
- `retain running 0;` keeps `running` across iterations (normally the PDV is
  reset each loop) and seeds it with 0.
- `running + amount;` is the **sum statement** — shorthand for an accumulating,
  auto-retained add.
- Explicit `output;` means *you* decide when rows are written; here, once per
  region.

## 6. Combining datasets: `merge`

A match-merge joins datasets on a `by` key. The `in=` flag tells you which
source contributed the row.

```sas
data names;  input id name $;  datalines;
1 John
2 Mary
3 Tim
;
run;

data scores; input id score; datalines;
1 95
2 85
4 70
;
run;

data matched;
  merge names(in=n) scores(in=s);
  by id;
  if n and s;        /* inner join: keep ids present in BOTH */
run;
```

`if n and s;` keeps only matched ids (an inner join). Use `if n;` for a left
join, `if n and not s;` for left-only rows, and so on. Both inputs must be sorted
by the `by` variables.

## 7. Informats and formats: reading and showing typed values

*Informats* parse non-trivial text on the way **in**; *formats* control how
values display on the way **out**.

```sas
data t;
  input id name $ pay : comma8. hired : date9.;
  format pay dollar10.2 hired date9.;
  datalines;
1 Anna 1,234 15JAN2020
2 Bob 56,789 01JUL2021
;
run;

proc print data=t;
run;
```

- `pay : comma8.` uses the `comma` **informat** to strip the thousands comma so
  `1,234` parses as the number `1234`.
- `hired : date9.` parses `15JAN2020` into a SAS date (days since 1960-01-01).
- The `format` statement attaches **output** formats: `pay` prints as
  `$1,234.00`, `hired` prints back as `15JAN2020`.

Dates, times, and datetimes also have literal syntax: `'15JAN2020'd`,
`'14:30:00't`, `'01JAN2020:14:30:00'dt`.

## 8. Summaries: PROC MEANS and PROC FREQ

```sas
proc means data=sales n mean min max;
  class region;
  var amount;
run;

proc freq data=sales;
  tables region;             /* one-way counts */
  tables region*product;     /* two-way cross-tab */
run;
```

- `PROC MEANS` reports N, mean, min, max (and more) per `class` group.
- `PROC FREQ` builds frequency tables; `a*b` cross-tabulates. Add `/ chisq` for a
  Pearson chi-square, or `/ list` for an n-way list layout.

## 9. PROC SQL

ASS speaks SQL over your datasets (backed by embedded SQLite):

```sas
proc sql;
  create table big as
    select region, sum(amount) as total
    from sales
    group by region
    having sum(amount) > 100
    order by total desc;
quit;

proc print data=big;
run;
```

Note PROC SQL ends with `quit;`, not `run;`. It supports `select`/`where`/`group
by`/`having`/`order by`/joins and `create table ... as`.

## 10. Macros

Macros generate code before it is parsed. Use `%let` for simple substitution and
`%macro` for reusable, parameterized blocks.

```sas
%let cutoff = 18;

%macro report(ds);
  proc print data=&ds;
  run;
%mend report;

data adults;
  input name $ age;
  if age >= &cutoff;
  datalines;
John 25
Tim 12
;
run;

%report(adults)
```

`&cutoff` expands to `18`; `%report(adults)` expands to a full PROC PRINT step.
Macros also support `%if/%then/%else` and `%do` loops.

## 11. Reading and writing files

Read external flat files with `infile`/`input`, write them with `file`/`put`:

```sas
data fromcsv;
  infile "in.csv" dsd firstobs=2;     /* dsd = CSV rules; skip header */
  input id name $ amount;
run;

data _null_;
  set fromcsv;
  file "out.txt" dlm=",";
  put id name amount;
run;
```

`data _null_;` runs the step for its side effects (here, writing a file) without
creating a dataset.

For spreadsheets and delimited files with headers, `PROC IMPORT` / `PROC EXPORT`
are higher-level:

```sas
proc import datafile="data.xlsx" out=work.d dbms=xlsx replace;
  getnames=yes;
run;

proc export data=work.d outfile="copy.csv" dbms=csv replace;
run;
```

## 12. Native SAS datasets and databases

Bind a **library** to a directory and read native `.sas7bdat` files:

```sas
libname mydata "/path/to/dir";
proc print data=mydata.customers;   /* reads dir/customers.sas7bdat */
run;
```

Or bind a library to a **database** and treat its tables as datasets — read and
write:

```sas
libname pg postgres "host=localhost dbname=app user=me";

proc print data=pg.orders;          /* read a DB table */
run;

data pg.summary;                     /* write one back */
  set work.totals;
run;
```

Postgres, SQL Server, Oracle, SQLite, and DB2 are supported. See
[`databases.md`](databases.md) for connection strings and the read/write/append
semantics.

## 13. Checking your understanding against the corpus

ASS ships a compatibility corpus of small, value-verified programs. Browse
`corpus/` for a worked example of nearly every feature, and run the harness:

```bash
ass test corpus/                    # run everything, report pass rates
ass test --feature data-step corpus/
ass test -v corpus/                 # show failure detail
```

Each corpus item is a directory with an `input.sas` and a `meta.yaml` that
declares the expected output values — a precise, runnable spec for the feature.

## Where to go next

- The [**Reference**](reference.md) — the complete implemented language surface.
- The [**Cookbook**](cookbook.md) — recipes for ETL, reshaping, file round-trips,
  user formats, frequencies, and regression.
- [`COMPATIBILITY.md`](COMPATIBILITY.md) — what "compatible" means here and which
  features are intentionally deferred.

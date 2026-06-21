# ASS Cookbook

Task-oriented recipes for **ASS — Analyst's Statistical Suite**. Each is a
complete, runnable program solving one real problem. Adapt the dataset names and
paths to your own.

New here? Read the [**Tutorial**](tutorial.md) first. For the full list of
statements, functions, and options each recipe uses, see the
[**Reference**](reference.md).

- [Filtering and deriving columns](#filtering-and-deriving-columns)
- [Inner / left / anti joins with merge](#inner--left--anti-joins-with-merge)
- [Per-group totals and running sums](#per-group-totals-and-running-sums)
- [Deduplicating rows](#deduplicating-rows)
- [Reshaping with arrays](#reshaping-with-arrays)
- [CSV in and out](#csv-in-and-out)
- [Excel round-trip](#excel-round-trip)
- [Cleaning text with user informats](#cleaning-text-with-user-informats)
- [Labeling groups with user formats](#labeling-groups-with-user-formats)
- [Frequencies and chi-square](#frequencies-and-chi-square)
- [Group statistics](#group-statistics)
- [SQL aggregation and joins](#sql-aggregation-and-joins)
- [Linear regression](#linear-regression)
- [Parameterizing with macros](#parameterizing-with-macros)
- [Reading native SAS and database tables](#reading-native-sas-and-database-tables)

---

## Filtering and deriving columns

Keep matching rows, add computed columns, and restrict the output:

```sas
data report;
  set sales;
  if amount > 0;                       /* subsetting if */
  net = amount * (1 - 0.07);
  if amount >= 1000 then tier = "big";
  else tier = "small";
  keep region amount net tier;
run;
```

> The subsetting `if` drops failing rows before `output`. Use `keep=`/`drop=` (or
> the `keep`/`drop` statements) to control which columns survive.

---

## Inner / left / anti joins with merge

Sort both inputs by the key, then `merge ... by`, and use `in=` flags to pick the
join type:

```sas
proc sort data=names  out=n; by id; run;
proc sort data=scores out=s; by id; run;

data inner;     merge n(in=a) s(in=b); by id; if a and b;        run; /* both    */
data leftjoin;  merge n(in=a) s(in=b); by id; if a;              run; /* keep n  */
data nomatch;   merge n(in=a) s(in=b); by id; if a and not b;    run; /* n only  */
```

---

## Per-group totals and running sums

```sas
proc sort data=sales out=s; by region; run;

data totals;
  set s;
  by region;
  if first.region then total = 0;     /* reset at group start */
  total + amount;                     /* sum statement: accumulate */
  if last.region then output;         /* one row per region */
  keep region total;
run;
```

For a running balance *within* groups, drop the `last.region` guard and `output`
every row.

---

## Deduplicating rows

Remove rows that repeat a key, keeping the first per key:

```sas
proc sort data=raw out=unique nodupkey;
  by customer_id;
run;
```

To dedupe on the entire row, sort by every variable with `nodupkey`. For
"keep the last per group," sort so the desired row is last and emit `if
last.key`.

---

## Reshaping with arrays

Turn four monthly columns into one normalized column per month (wide → long):

```sas
data long;
  set wide;                            /* has q1 q2 q3 q4 */
  array q{4} q1-q4;
  do i = 1 to 4;
    quarter = i;
    value = q{i};
    output;                            /* one row per quarter */
  end;
  keep id quarter value;
run;
```

The `q1-q4` numbered range names the four variables; `array q{4}` indexes them.

---

## CSV in and out

Read a CSV with a header line, then write a transformed CSV:

```sas
data fromcsv;
  infile "in.csv" dsd firstobs=2;      /* dsd = CSV rules; skip header row */
  input id name $ amount;
run;

data _null_;
  set fromcsv;
  file "out.csv" dsd;                  /* dsd quotes values containing commas */
  if _n_ = 1 then put "id,name,amount";/* write a header once */
  put id name amount;
run;
```

Or use the higher-level PROCs when the file has a header you want auto-detected:

```sas
proc import datafile="in.csv" out=work.d dbms=csv replace;
  getnames=yes;
run;

proc export data=work.d outfile="out.csv" dbms=csv replace;
run;
```

For tab- or custom-delimited files use `dbms=tab` or `dbms=dlm` with
`delimiter="|"`.

---

## Excel round-trip

Export to a worksheet and read it back (the `.xlsx` engine is dependency-free):

```sas
proc export data=work.report outfile="report.xlsx" dbms=xlsx replace;
run;

proc import datafile="report.xlsx" out=work.back dbms=xlsx replace;
  getnames=yes;
run;
```

IMPORT sniffs column types — character columns come back character, numeric
columns numeric.

---

## Cleaning text with user informats

Map coded text to numbers as data is read, using `PROC FORMAT INVALUE`:

```sas
proc format;
  invalue yn "Y"=1 "Yes"=1 "N"=0 "No"=0;
run;

data clean;
  input id consent : yn.;              /* "Yes"/"N" -> 1/0 on input */
  datalines;
1 Yes
2 N
3 Y
;
run;
```

---

## Labeling groups with user formats

Define value labels once and reuse them for display and grouping:

```sas
proc format;
  value agegrp low-17="minor" 18-64="adult" 65-high="senior";
  value $reg  "N"="North" "S"="South";
run;

data people;
  input name $ region $ age;
  format age agegrp. region $reg.;     /* attach the formats */
  datalines;
Ann N 30
Bo  S 70
Cy  N 12
;
run;

proc print data=people; run;          /* shows "adult", "North", ... */

proc freq data=people;
  tables age;                          /* groups by the formatted bucket */
run;
```

`PROC FREQ` and `PROC MEANS` group by the *formatted* value, so `agegrp.` yields
minor/adult/senior counts rather than one row per age.

---

## Frequencies and chi-square

```sas
proc freq data=sales;
  tables region;                       /* one-way counts + percents */
  tables region*product / chisq;       /* cross-tab + Pearson chi-square */
  tables region*product*year / list;   /* n-way list layout */
run;
```

Trim the display with options: `tables a*b / nopercent nocum;`.

---

## Group statistics

```sas
proc means data=sales n mean stddev min max;
  class region;
  var amount;
run;
```

Use `by region;` instead of `class` when the data is already sorted by region and
you want one report block per group. `PROC SUMMARY` is the same engine.

---

## SQL aggregation and joins

```sas
proc sql;
  create table by_region as
    select region,
           count(*)    as orders,
           sum(amount) as total,
           avg(amount) as avg_amt
    from sales
    group by region
    having sum(amount) > 100
    order by total desc;

  create table enriched as
    select s.*, r.manager
    from sales as s
    join regions as r on s.region = r.region;
quit;
```

PROC SQL ends with `quit;`. Sources can be WORK datasets or a database libref.

---

## Linear regression

```sas
proc reg data=xy;
  model y = x1 x2;                     /* estimates, std err, t, Pr>|t|, R^2 */
run;

proc glm data=trial;
  class treatment;                     /* categorical predictor */
  model response = treatment dose;
run;
```

`PROC GLM` encodes `class` variables with reference-cell coding.

---

## Parameterizing with macros

Generate repeated steps from a single definition:

```sas
%macro summarize(ds, byvar, measure);
  proc means data=&ds n mean min max;
    class &byvar;
    var &measure;
  run;
%mend summarize;

%summarize(sales, region, amount)
%summarize(sales, product, amount)
```

Loop over a list with `%do`:

```sas
%macro printall(prefix);
  %do i = 1 %to 3;
    proc print data=&prefix&i; run;
  %end;
%mend printall;

%printall(part)          /* prints part1, part2, part3 */
```

---

## Reading native SAS and database tables

Native `.sas7bdat` from a directory library:

```sas
libname legacy "/data/exports";
proc print data=legacy.customers;     /* reads /data/exports/customers.sas7bdat */
run;
```

A database table as a dataset, and writing one back:

```sas
libname pg postgres "host=localhost dbname=app user=me";

data work.recent;
  set pg.orders(where=(amount > 0));   /* read + filter from the DB */
run;

data pg.summary;                       /* write a new DB table */
  set work.by_region;
run;

proc append base=pg.fact data=work.delta;  /* in-place INSERT */
run;
```

Connection strings for Postgres, SQL Server, Oracle, SQLite, and DB2 are in
[`databases.md`](databases.md).

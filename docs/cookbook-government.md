# ASS Cookbook — Government, Public Health & Healthcare

Task-oriented recipes for **ASS — Analyst's Statistical Suite** aimed at the
public sector: surveillance counts, demographic cross-tabs, enrollment and claims
ETL, incidence rates, survey tabulation, and data-quality gating for data
submissions. Each program is complete and runnable; adapt names and paths to your
own data. All data shown here is small, synthetic, and illustrative — no real
records or PII.

New here? Read the [**Tutorial**](tutorial.md) first, see the general
[**Cookbook**](cookbook.md) for non-domain recipes, and consult the
[**Reference**](reference.md) for the full statement/function/option surface.

- [Demographic cross-tabs with chi-square](#demographic-cross-tabs-with-chi-square)
- [Banding with user formats, then grouped statistics](#banding-with-user-formats-then-grouped-statistics)
- [Case-count surveillance by period](#case-count-surveillance-by-period)
- [Incidence rates per 100,000](#incidence-rates-per-100000)
- [Enrollment / eligibility ETL](#enrollment--eligibility-etl)
- [Counting events per person](#counting-events-per-person)
- [Survey-style tabulation](#survey-style-tabulation)
- [Summary statistics by stratum](#summary-statistics-by-stratum)
- [A public reporting listing](#a-public-reporting-listing)
- [Data-quality gating with PROC PROOF](#data-quality-gating-with-proc-proof)

---

## Demographic cross-tabs with chi-square

Cross-tabulate age group by region and test for association:

```sas
proc format;
  value agegrp low-17="0-17" 18-44="18-44" 45-64="45-64" 65-high="65+";
  value $reg "N"="North" "S"="South" "E"="East" "W"="West";
run;

data persons;
  input id region $ age sex $;
  format age agegrp. region $reg.;
  datalines;
1 N 12 F
2 N 34 M
3 S 70 F
4 S 55 M
5 E 22 F
6 E 9  M
7 W 67 F
8 W 41 M
9 N 50 M
10 S 30 F
11 E 80 M
12 W 15 F
;
run;

proc freq data=persons;
  tables region*age / chisq;
run;
```

The cells group by the *formatted* value, so `agegrp.` collapses raw ages into
bands. The chi-square footer reports the Pearson statistic and its p-value:

```
Statistics for Table of region by age

Chi-Square  DF=9  Value=4.0000  Prob=0.9114
```

> `/ chisq` adds the Pearson test. Display trimmers (`nofreq`, `nopercent`,
> `nocum`, `norow`, `nocol`) shape one-way tables; for the dense two-way layout
> reach for them on the one-way `tables` instead.

---

## Banding with user formats, then grouped statistics

Bucket a continuous measure (income) into low/middle/high and summarize another
variable within each band — no extra columns needed, because FREQ and MEANS group
by the formatted value:

```sas
proc format;
  value incband low-24999="low" 25000-74999="middle" 75000-high="high";
run;

data households;
  input id income age;
  format income incband.;
  datalines;
1 18000 40
2 52000 33
3 90000 61
4 23000 29
5 67000 45
6 120000 50
7 30000 38
8 8000 70
;
run;

proc means data=households n mean min max;
  class income;
  var age;
run;
```

```
Obs  income  Variable  N          Mean        StdDev  Min  Max
  1  low     age       3  46.333333333  21.221058723   29   70
  2  middle  age       3  38.666666667  6.0277137733   33   45
  3  high    age       2          55.5  7.7781745931   50   61
```

> The `low-`/`-high` open ranges catch every value. Swap `proc means` for `proc
> freq; tables income;` to get counts per band instead of statistics.

---

## Case-count surveillance by period

Roll event dates up to a reporting period with the date functions, then count.
`intnx('month', d, 0)` snaps each onset date to the first of its month:

```sas
data cases;
  input caseid onset : date9.;
  month = intnx('month', onset, 0);   /* first day of onset month */
  format month date9.;
  datalines;
1 03JAN2024
2 17JAN2024
3 02FEB2024
4 28FEB2024
5 14FEB2024
6 05MAR2024
7 19MAR2024
8 30MAR2024
9 22MAR2024
;
run;

proc freq data=cases;
  tables month / nocum;
run;
```

```
Obs  month      Frequency  Percent
  1  01JAN2024          2     22.2
  2  01FEB2024          3     33.3
  3  01MAR2024          4     44.4
```

> Use `intnx('qtr', onset, 0)` for quarterly buckets or `intnx('year', onset, 0)`
> for annual; `intck('week', start, onset)` gives an integer week offset for an
> epi-week index.

---

## Incidence rates per 100,000

Join period case counts to a population denominator and compute a rate. PROC SQL
does the join and arithmetic in one pass:

```sas
data cases;
  input region $ cases;
  datalines;
North 42
South 18
East  31
West  27
;
run;

data pop;
  input region $ population;
  datalines;
North 210000
South 150000
East  175000
West  130000
;
run;

proc sql;
  create table rates as
    select c.region,
           c.cases,
           p.population,
           c.cases / p.population * 100000 as rate_per_100k
    from cases as c
    join pop as p on c.region = p.region
    order by rate_per_100k desc;
quit;

data rates;
  set rates;
  format rate_per_100k 8.1 population comma10.;
run;

proc print data=rates noobs; run;
```

```
region  cases  population  rate_per_100k
West       27     130,000           20.8
North      42     210,000           20.0
East       31     175,000           17.7
South      18     150,000           12.0
```

> A second `data` step attaches the display formats, because user formats aren't
> applied to PROC SQL *output* columns. The same join works in a DATA step
> (`merge cases pop; by region;`) if you'd rather avoid SQL.

---

## Enrollment / eligibility ETL

Read a coded enrollment extract, decode the code columns to readable labels while
reading (`PROC FORMAT INVALUE`), and derive eligibility flags:

```sas
proc format;
  invalue $covdec  "M"="Medicare" "C"="CHIP" "P"="Private" other="Unknown";
  invalue $statdec "A"="Active" "T"="Terminated" "P"="Pending" other="?";
run;

data enroll;
  infile "/tmp/gov/in.csv" dsd firstobs=2;     /* skip the header row */
  input id coverage : $covdec. status : $statdec. age;
  if status = "Active" then active = 1; else active = 0;
  if age >= 65 then senior = 1; else senior = 0;
run;

proc print data=enroll noobs; run;
```

With `in.csv` holding `id,coverage,status,age` / `1001,M,A,67` / …:

```
  id  coverage  status      age  active  senior
1001  Medicare  Active       67       1       1
1002  CHIP      Active        5       1       0
1003  Medicare  Terminated   72       0       1
1004  Private   Active       40       1       0
1005  CHIP      Pending      12       0       0
```

> `invalue $name` maps coded text to text; `invalue name` (no `$`) maps to
> numbers (e.g. `"Y"=1 "N"=0`). The `:` in `input` applies the informat to a
> delimited field. Use `proc import ... dbms=csv` instead when you'd rather sniff
> the columns automatically.

---

## Counting events per person

Match a person/enrollee file to an events or claims file by id. The reliable
pattern is **roll the many-side up first, then left-join** — it keeps people with
zero events and avoids the pitfalls of accumulating across an unequal merge:

```sas
data persons;
  input id name $ region $;
  datalines;
1 Ann North
2 Bo  South
3 Cy  East
4 Di  West
;
run;

data events;
  input id claim_amt;
  datalines;
1 120
1 80
2 50
3 200
3 75
3 30
;
run;

/* Step 1: roll events up to one row per id */
proc sort data=events out=e; by id; run;

data evsum;
  set e;
  by id;
  if first.id then do; n_events = 0; total_amt = 0; end;
  n_events + 1;
  total_amt + claim_amt;
  if last.id then output;
  keep id n_events total_amt;
run;

/* Step 2: left-join persons to the rollup */
proc sort data=persons out=p; by id; run;

data perevent;
  merge p(in=inp) evsum;
  by id;
  if inp;                                   /* keep every person */
  if n_events = . then do; n_events = 0; total_amt = 0; end;
run;

proc print data=perevent noobs; run;
```

With four people (ids 1–4) and six events (2 for id 1, 1 for id 2, 3 for id 3):

```
id  name  region  n_events  total_amt
 1  Ann   North          2        200
 2  Bo    South          1         50
 3  Cy    East           3        305
 4  Di    West           0          0
```

> `if inp;` keeps people with no events; the missing-value guard zeroes their
> counts. For a list of person-event pairs instead of a rollup, skip step 1 and
> `merge p(in=inp) e(in=ine); by id; if inp;`.

---

## Survey-style tabulation

Frequencies and percentages of a coded categorical response, decoded for display:

```sas
proc format;
  value $resp "1"="Strongly agree" "2"="Agree" "3"="Neutral"
              "4"="Disagree" "5"="Strongly disagree";
run;

data survey;
  input respid satisfaction $;
  format satisfaction $resp.;
  datalines;
1 1
2 2
3 2
4 3
5 1
6 4
7 2
8 5
9 3
10 2
;
run;

proc freq data=survey;
  tables satisfaction / nocum;
run;
```

```
Obs  satisfaction       Frequency  Percent
  1  Strongly agree             2     20.0
  2  Agree                      4     40.0
  3  Neutral                    2     20.0
  4  Disagree                   1     10.0
  5  Strongly disagree          1     10.0
```

> Cross two questions with `tables q1*q2;` for a contingency table, and add
> `/ chisq` to test their independence.

---

## Summary statistics by stratum

Pre-sort, then report one block per stratum with `by`:

```sas
data visits;
  input region $ sex $ los;       /* los = length of stay (days) */
  datalines;
North F 3
North M 5
North F 2
South F 7
South M 4
South M 6
East  F 1
East  M 8
;
run;

proc sort data=visits out=v; by region sex; run;

proc means data=v n mean min max;
  by region sex;
  var los;
run;
```

```
Obs  region  sex  Variable  N  Mean        StdDev  Min  Max
  1  East    F    los       1     1             .    1    1
  2  East    M    los       1     8             .    8    8
  3  North   F    los       2   2.5  0.7071067812    2    3
  4  North   M    los       1     5             .    5    5
  5  South   F    los       1     7             .    7    7
  6  South   M    los       2     5  1.4142135624    4    6
```

> `class region sex;` gives the same strata without pre-sorting, in one combined
> table. `PROC SUMMARY` is the same engine.

---

## A public reporting listing

A presentation-ready listing with labels, formats, a title, and a footnote:

```sas
proc format;
  value $reg "N"="Northern" "S"="Southern";
run;

data report;
  input region $ cases population;
  rate = cases / population * 100000;
  format region $reg. rate 8.1 population comma10. cases comma8.;
  label region     = "Health Region"
        cases      = "Reported Cases"
        population = "Population"
        rate       = "Rate per 100,000";
  datalines;
N 42 210000
S 18 150000
;
run;

title "Quarterly Disease Surveillance Report";
footnote "Source: synthetic illustrative data";

proc print data=report noobs label;
  var region cases population rate;
run;
```

```
Quarterly Disease Surveillance Report

Health Region  Reported Cases  Population  Rate per 100,000
Northern                   42     210,000              20.0
Southern                   18     150,000              12.0

Source: synthetic illustrative data
```

> `label` switches headers to the variable labels; `format` controls how each
> column renders. `var` fixes column order.

---

## Data-quality gating with PROC PROOF

Public-sector data submissions usually have to pass acceptance rules before they
can be filed. `PROC PROOF` declares those rules, writes the offending rows to an
`out=` dataset, and — with `severity=error` — makes the process **exit non-zero**
so a submission pipeline can fail the batch:

```sas
data persons;                                    /* the reference (parent) file */
  input personid region $ age;
  datalines;
1 N 34
2 S 50
3 E 200
4 W 22
5 N 41
;
run;

data events;                                     /* event 103 points at a missing person */
  input eventid personid claim_amt;
  datalines;
100 1 250
101 2 1200
102 5 500
103 9 40
;
run;

proc proof data=events out=bad severity=error;
  require eventid personid claim_amt;            /* required fields exist */
  notnull personid claim_amt;                    /* no missing keys/values */
  unique eventid;                                /* unique event id */
  range claim_amt 0 - 100000;                    /* plausible amount */
  key personid references persons(personid);     /* event maps to a person */
run;

proc proof data=persons severity=error;
  unique personid;
  values region in ("N" "S" "E" "W");            /* valid code domain */
  range age 0 - 120;                             /* age range rule */
run;

proc print data=bad noobs; run;
```

With an event whose `personid=9` has no matching person and a person aged 200:

```
PROC PROOF — WORK.EVENTS (4 obs)

  Assertion                                Result Violations/Checked
  require eventid personid claim_amt       PASS   0/1
  notnull personid claim_amt               PASS   0/4
  unique eventid                           PASS   0/4
  range claim_amt 0 - 100000               PASS   0/4
  key personid references persons(personi… FAIL   1/4
      offending obs: 4
ERROR: PROC PROOF: assertion failed (error): key personid references ... — 1/4 rows.

PROC PROOF — WORK.PERSONS (5 obs)
  ...
  range age 0 - 120                        FAIL   1/5
      offending obs: 3
ERROR: PROC PROOF: assertion failed (error): range age 0 - 120 — 1/5 rows.

eventid  personid  claim_amt  _rule_                                     _obs_
    103         9         40  key personid references persons(personid)      4
```

The run logs `ERROR` for each failure and the CLI exits 1, so
`ass submit.sas && file_it.sh` won't file a batch that fails referential
integrity, a code-domain check, or a range rule. The `out=bad` dataset has one
row per (source row × failed assertion), annotated with `_rule_` and `_obs_` for
triage.

> `severity=warn` on an individual assertion logs a `WARNING` without affecting
> the exit code — use it for advisory checks. A foreign key with any missing
> component passes, mirroring SQL NULL-FK semantics. The full assertion catalogue
> (`require`, `type`, `notnull`, `values`, `range`, `unique`, `key … references`,
> `rule`) is documented in [`proofing.md`](proofing.md).

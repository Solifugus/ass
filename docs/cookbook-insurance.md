# ASS Cookbook — Insurance / Actuarial

Task-oriented recipes for **ASS — Analyst's Statistical Suite** aimed at
insurance and actuarial work: premium, exposure, losses, claim triangles, rate
tables, and book-of-business data quality. Each recipe is a complete, runnable
program; the data is small and synthetic so you can see every value. Adapt the
dataset names and paths to your own.

New here? Read the [**Tutorial**](tutorial.md) first, and the general
[**Cookbook**](cookbook.md) for the core patterns these recipes build on. For the
full list of statements, functions, and options, see the
[**Reference**](reference.md).

- [Pro-rating earned and unearned premium](#pro-rating-earned-and-unearned-premium)
- [Loss ratio by line of business](#loss-ratio-by-line-of-business)
- [Building a loss-development triangle](#building-a-loss-development-triangle)
- [Merging policies and claims](#merging-policies-and-claims)
- [Claim frequency and severity](#claim-frequency-and-severity)
- [Rate-table lookup and expected premium](#rate-table-lookup-and-expected-premium)
- [Open/closed reserve rollups](#openclosed-reserve-rollups)
- [Banding driver age with user formats](#banding-driver-age-with-user-formats)
- [Proofing a book of business](#proofing-a-book-of-business)

---

## Pro-rating earned and unearned premium

Split written premium into earned and unearned as of a valuation date, using
exposure-period date math:

```sas
data earned;
  input policy $ written start date9. end date9. asof date9.;
  days_total = end - start + 1;
  earned_end = min(asof, end);
  if earned_end < start then days_earned = 0;
  else days_earned = earned_end - start + 1;
  earned   = round(written * days_earned / days_total, 0.01);
  unearned = round(written - earned, 0.01);
  format start end asof date9.;
  datalines;
P001 1200 01JAN2024 31DEC2024 30JUN2024
P002  600 01APR2024 30SEP2024 30JUN2024
P003  900 01JUL2024 31DEC2024 30JUN2024
;
run;

proc print data=earned noobs;
  var policy written days_total days_earned earned unearned;
run;
```

```
policy  written  days_total  days_earned  earned  unearned
P001       1200         366          182  596.72    603.28
P002        600         183           91  298.36    301.64
P003        900         184            0       0       900
```

> A SAS date is a day count, so `end - start + 1` is the exposure length in days
> and pro-rating is plain arithmetic. P003 starts after the valuation date, so
> nothing is earned yet. Read dates with the `date9.` informat (formatted input,
> no colon); attach `format ... date9.` for readable display.

---

## Loss ratio by line of business

Aggregate losses and premium per line of business and report the ratio as a
percent:

```sas
data claims;
  input lob $ premium losses;
  datalines;
auto 1000 720
auto  800 500
home 1500 900
home 1200 700
life 2000 300
;
run;

proc sql;
  create table lr as
    select lob,
           sum(premium)             as premium,
           sum(losses)              as losses,
           sum(losses)/sum(premium) as loss_ratio
    from claims
    group by lob
    order by loss_ratio desc;
quit;

data lr; set lr; format loss_ratio percent8.1; run;
proc print data=lr noobs; run;
```

```
lob   premium  losses  loss_ratio
auto     1800    1220       67.8%
home     2700    1600       59.3%
life     2000     300       15.0%
```

> PROC SQL does the grouped division; a one-line follow-up DATA step attaches the
> `percent8.1` format (ASS does not yet apply user/display formats to SQL *output*
> columns, so format after the fact). For an all-DATA-step version, see
> [Per-group totals](cookbook.md#per-group-totals-and-running-sums).

---

## Building a loss-development triangle

Reshape long cumulative-paid records (accident year × development lag) into the
classic wide triangle layout, using a DATA step + array — no PROC TRANSPOSE
needed:

```sas
data tri_long;
  input ay lag paid;       /* accident year, development lag, cumulative paid */
  datalines;
2021 1 400
2021 2 650
2021 3 800
2022 1 450
2022 2 700
2023 1 500
;
run;

proc sort data=tri_long out=s; by ay lag; run;

data triangle;
  set s;
  by ay;
  retain d1 d2 d3;
  array d{3} d1-d3;
  if first.ay then do i = 1 to 3; d{i} = .; end;
  d{lag} = paid;                       /* drop each lag into its column */
  if last.ay then output;              /* one row per accident year     */
  keep ay d1 d2 d3;
run;

proc print data=triangle noobs;
  var ay d1 d2 d3;
run;
```

```
ay    d1   d2   d3
2021  400  650  800
2022  450  700    .
2023  500    .    .
```

> The array `d1-d3` holds the development columns; `d{lag} = paid` places each
> long row into the right column, and `first./last.` resets and emits once per
> accident year. The lower-right triangle stays missing — the part actuaries
> project. To go wide → long instead, see
> [Reshaping with arrays](cookbook.md#reshaping-with-arrays).

---

## Merging policies and claims

Match claims to policies by id, count claims per policy, and keep policies with
no claims (a left join via `merge ... by` and `in=`):

```sas
data policies;
  input policy $ lob $ premium;
  datalines;
P001 auto 1000
P002 home 1500
P003 life 2000
;
run;

data claims;
  input policy $ amount;
  datalines;
P001 300
P001 250
P002 900
;
run;

proc sql;                              /* claim count + paid per policy */
  create table cc as
    select policy, count(*) as n_claims, sum(amount) as paid
    from claims group by policy;
quit;

proc sort data=policies out=p; by policy; run;
proc sort data=cc        out=c; by policy; run;

data joined;
  merge p(in=inp) c(in=inc);
  by policy;
  if inp;                              /* keep every policy            */
  if not inc then do; n_claims = 0; paid = 0; end;  /* fill zero-claim */
  has_claim = inc;
run;

proc print data=joined noobs; run;
```

```
policy  lob   premium  n_claims  paid  has_claim
P001    auto     1000         2   550          1
P002    home     1500         1   900          1
P003    life     2000         0     0          0
```

> `in=` flags which input supplied the row: `if inp` keeps all policies (left
> join), and `if not inc` fills zero for policies with no claims. For inner/anti
> join variants, see [joins with merge](cookbook.md#inner--left--anti-joins-with-merge).

---

## Claim frequency and severity

Count claims by cause (frequency) and summarize amounts by cause (severity) in
one pass each:

```sas
data claims;
  input cause $ amount;
  datalines;
collision 3200
collision 1500
collision 800
theft 5000
theft 4200
weather 900
weather 1100
weather 700
;
run;

proc freq data=claims;
  tables cause / nocum;          /* claim frequency by cause */
run;

proc means data=claims n mean stddev min max;
  class cause;                   /* severity stats by cause  */
  var amount;
run;
```

```
Obs  cause      Frequency  Percent
  1  collision          3     37.5
  2  theft              2     25.0
  3  weather            3     37.5

Obs  cause      Variable  N   Mean        StdDev        Min   Max
  1  collision  amount    3   1833.33...  1234.23...    800   3200
  2  theft      amount    2   4600        565.68...     4200  5000
  3  weather    amount    3   900         200           700   1100
```

> PROC FREQ gives claim *frequency* (counts and share); PROC MEANS gives claim
> *severity* (mean/min/max amount per cause). Together they are the two halves of
> a pure-premium decomposition.

---

## Rate-table lookup and expected premium

Join exposures to a rate table keyed on territory and class, then compute the
expected premium:

```sas
data rates;                       /* rate per $1000 of coverage */
  input terr $ class $ rate;
  datalines;
A pref 4.5
A std  6.0
B pref 5.5
B std  7.5
;
run;

data exposures;
  input policy $ terr $ class $ coverage;   /* coverage in $1000 units */
  datalines;
P001 A pref 200
P002 B std  150
P003 A std  300
;
run;

proc sort data=rates     out=r; by terr class; run;
proc sort data=exposures out=e; by terr class; run;

data priced;
  merge e(in=ine) r(in=inr);
  by terr class;
  if ine;                                   /* keep every exposure */
  premium = round(coverage * rate, 0.01);
run;

proc print data=priced noobs;
  var policy terr class coverage rate premium;
run;
```

```
policy  terr  class  coverage  rate  premium
P001    A     pref        200   4.5      900
P003    A     std         300     6     1800
P002    B     std         150   7.5     1125
```

> A multi-key `merge ... by terr class` is a clean rate lookup. The same join in
> PROC SQL (`from exposures e join rates r on e.terr=r.terr and e.class=r.class`)
> works too; use whichever reads better for your pipeline.

---

## Open/closed reserve rollups

Roll case reserves and paid amounts up to the line of business, counting open
claims, with `first.`/`last.` and the sum statement:

```sas
data claims;
  input lob $ status $ reserve paid;
  datalines;
auto open  5000 1000
auto open  2000  500
auto closed   0 3000
home open  8000 2000
home closed   0 4500
;
run;

proc sort data=claims out=s; by lob; run;

data reserves;
  set s;
  by lob;
  retain n_open tot_reserve tot_paid;
  if first.lob then do;
    n_open = 0; tot_reserve = 0; tot_paid = 0;
  end;
  if status = "open" then n_open + 1;
  tot_reserve + reserve;               /* sum statement: auto-retain */
  tot_paid    + paid;
  if last.lob then output;             /* one row per line of business */
  keep lob n_open tot_reserve tot_paid;
run;

proc print data=reserves noobs;
  var lob n_open tot_reserve tot_paid;
run;
```

```
lob   n_open  tot_reserve  tot_paid
auto       2         7000      4500
home       1         8000      6500
```

> `first.lob` resets the accumulators, the sum statement (`var + expr;`)
> accumulates with missing-as-zero, and `last.lob` emits one summary row per
> group. Incurred = `tot_reserve + tot_paid` if you want it as a derived column.

---

## Banding driver age with user formats

Bucket continuous ages into rating bands once with `PROC FORMAT`, then group by
the band in FREQ and MEANS:

```sas
proc format;
  value ageband
    low-24  = "16-24"
    25-39   = "25-39"
    40-64   = "40-64"
    65-high = "65+";
run;

data drivers;
  input age premium claims;
  format age ageband.;                 /* attach the band format */
  datalines;
19 1800 2
22 1600 1
31 1100 0
45  900 1
52  950 0
70 1300 1
68 1250 2
;
run;

proc freq data=drivers;
  tables age / nocum;                  /* counts per age band */
run;

proc means data=drivers n mean;
  class age;                           /* avg premium/claims per band */
  var premium claims;
run;
```

```
Obs  age    Frequency  Percent
  1  16-24          2     28.6
  2  25-39          1     14.3
  3  40-64          2     28.6
  4  65+            2     28.6

Obs  age    Variable  N  Mean  ...
  1  16-24  premium   2  1700
  1  16-24  claims    2   1.5
  ...
```

> FREQ and MEANS group by the *formatted* value, so one `ageband.` definition
> drives both reports — change the bands once and every grouping follows. The same
> trick works for territory or vehicle-class codes with a `value $...` format.

---

## Proofing a book of business

Validate a policy extract before it feeds rating or reserving: unique policy ids,
present premium/exposure, no negative premium, exposure in `[0,1]`, and every
line of business referencing a known LOB. `PROC PROOF` is an ASS value-add (not a
SAS PROC):

```sas
data lobs;
  input lob $;
  datalines;
auto
home
life
;
run;

data policies;
  input policy $ lob $ premium exposure;
  datalines;
P001 auto 1000 1.0
P002 home 1500 0.5
P003 life 2000 1.0
P004 auto  -50 1.0
P005 boat 1200 1.0
P001 auto  800 1.0
;
run;

proc proof data=policies out=bad;
  require policy lob premium exposure;
  type premium=num exposure=num policy=char;
  notnull premium exposure;
  unique policy;                          /* no duplicate policy ids    */
  range premium >= 0;                     /* premium cannot be negative */
  range exposure 0 - 1;                   /* exposure in [0,1]          */
  key lob references lobs(lob);           /* lob must be a known LOB    */
run;

proc print data=bad noobs; run;
```

```
PROC PROOF — WORK.POLICIES (6 obs)

  Assertion                            Result Violations/Checked
  require policy lob premium exposure  PASS   0/1
  type premium=num exposure=num poli…  PASS   0/3
  notnull premium exposure             PASS   0/6
  unique policy                        FAIL   2/6
      offending obs: 1 6
  range premium >= 0                   FAIL   1/6
      offending obs: 4
  range exposure 0 - 1                 PASS   0/6
  key lob references lobs(lob)         FAIL   1/6
      offending obs: 5
ERROR: PROC PROOF: 3 error-level assertion(s) failed on WORK.POLICIES.

policy  lob   premium  exposure  _rule_                       _obs_
P001    auto     1000         1  unique policy                    1
P004    auto      -50         1  range premium >= 0               4
P005    boat     1200         1  key lob references lobs(lob)     5
P001    auto      800         1  unique policy                    6
```

> The `out=bad` dataset has one row per (source row × failed assertion), tagged
> with `_rule_` and `_obs_`, so violations are trivially filterable and routable.
> Error-level failures log `ERROR` and make `ass` exit non-zero — drop the program
> into a pipeline and a bad extract fails the build instead of flowing downstream.

For the full assertion catalog (`require`, `type`, `notnull`, `values`, `range`,
`unique`, `key … references`, `rule`), severities, and exit-code semantics, see
[**proofing.md**](proofing.md).

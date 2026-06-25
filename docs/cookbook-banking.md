# ASS Cookbook — Banking & Financial Services

Task-oriented recipes for **ASS — Analyst's Statistical Suite**, focused on
common **banking / financial-services** workloads: delinquency reporting,
transaction ETL, credit scorecards, vintage/cohort analysis, portfolio rollups,
loss ratios, and data-quality gating. Each recipe is a complete, runnable
program; adapt the dataset names, paths, and (illustrative) weights and
thresholds to your own data.

All sample data below is small, synthetic, and invented for illustration — there
is no real customer data here. New to ASS? Read the [**Tutorial**](tutorial.md)
first, see the general [**Cookbook**](cookbook.md) for non-domain recipes, and
the [**Reference**](reference.md) for the full list of statements, functions, and
options each recipe uses.

- [Delinquency bucketing and aging counts](#delinquency-bucketing-and-aging-counts)
- [Transaction ETL and per-account aggregation](#transaction-etl-and-per-account-aggregation)
- [A simple credit scorecard](#a-simple-credit-scorecard)
- [Vintage / cohort analysis](#vintage--cohort-analysis)
- [Account-to-activity left join](#account-to-activity-left-join)
- [Running balance per account](#running-balance-per-account)
- [Portfolio rollup with PROC SQL](#portfolio-rollup-with-proc-sql)
- [Loss given default and loss-rate reporting](#loss-given-default-and-loss-rate-reporting)
- [Reconciliation and data-quality gating](#reconciliation-and-data-quality-gating)
- [Parameterizing a delinquency rollup with macros](#parameterizing-a-delinquency-rollup-with-macros)
- [Reading from a banking database](#reading-from-a-banking-database)

---

## Delinquency bucketing and aging counts

Classify accounts into standard days-past-due (DPD) aging buckets with a user
`VALUE` format, then count them with `PROC FREQ`:

```sas
proc format;
  value dpdbucket low--1   = "unknown"
                  0        = "current"
                  1-29     = "1-29 DPD"
                  30-59    = "30-59 DPD"
                  60-89    = "60-89 DPD"
                  90-high  = "90+ DPD";
run;

data loans;
  input acct_id $ balance dpd;
  format balance dollar12.2 dpd dpdbucket.;
  datalines;
A100 5200.00 0
A101 1830.50 12
A102 9900.00 45
A104 6700.00 72
A105 1200.00 118
A107  890.00 95
;
run;

proc print data=loans; run;

proc freq data=loans;
  tables dpd;                           /* groups by the formatted bucket */
run;
```

> `PROC FREQ` groups by the *formatted* value, so the `dpd` column collapses into
> aging buckets (`current`, `1-29 DPD`, …) rather than one row per raw DPD value.
> The `low--1` range catches negative/sentinel values as "unknown".

---

## Transaction ETL and per-account aggregation

Read a CSV of transactions, derive debit/credit flags, then aggregate to one row
per account with `PROC SQL`:

```sas
/* write a synthetic transactions CSV to read back */
data _null_;
  file "txns.csv" dsd;
  put "acct_id,txn_date,amount,channel";
  put "A100,2026-01-03,-42.50,POS";
  put "A100,2026-01-05,1200.00,ACH";
  put "A100,2026-01-09,-89.99,POS";
  put "A101,2026-01-02,-15.00,ATM";
  put "A101,2026-01-20,-260.00,POS";
run;

data txns;
  infile "txns.csv" dsd firstobs=2;     /* skip the header row */
  input acct_id $ txn_date : yymmdd10. amount channel $;
  is_debit  = (amount < 0);
  is_credit = (amount > 0);
  format txn_date date9.;
run;

proc sql;
  create table acct_summary as
    select acct_id,
           count(*)                                        as n_txns,
           sum(amount)                                     as net_flow,
           sum(case when amount<0 then -amount else 0 end) as debits,
           sum(case when amount>0 then  amount else 0 end) as credits
    from txns
    group by acct_id
    order by acct_id;
quit;

proc print data=acct_summary noobs; run;
```

> `yymmdd10.` parses `2026-01-03` into a SAS date on input. The SQL `case`
> expression splits signed amounts into debit/credit totals in one pass. For a
> file that already has a header you want auto-detected, use
> `proc import datafile="txns.csv" dbms=csv ... getnames=yes;` instead.

---

## A simple credit scorecard

Score applicants as a generic weighted sum of attributes, clamp to a valid
range, then band the score with a user format. The weights here are purely
illustrative:

```sas
proc format;
  value scoreband low-579   = "Poor"
                  580-669   = "Fair"
                  670-739   = "Good"
                  740-799   = "Very Good"
                  800-high  = "Excellent";
run;

data applicants;
  input id $ util pct_ontime inquiries yrs_history;
  score = 650
        + (-200) * util          /* utilization hurts          */
        + ( 150) * pct_ontime    /* on-time history helps       */
        + ( -12) * inquiries     /* hard inquiries hurt         */
        + (   4) * yrs_history;  /* length of history helps     */
  score = round(score, 1);
  if score < 300 then score = 300;
  if score > 850 then score = 850;
  datalines;
P1 0.10 0.99 1 12
P2 0.85 0.70 6 2
P3 0.45 0.92 2 8
P4 0.30 0.85 0 5
P5 0.95 0.55 9 1
;
run;

proc print data=applicants noobs;
  var id score;
run;

/* same score shown as a band by attaching the format on display */
proc print data=applicants noobs label;
  label score="Risk band";
  var id score;
  format score scoreband.;
run;

proc freq data=applicants;
  tables score / nocum;                 /* counts per band */
  format score scoreband.;
run;
```

> Keep the raw numeric `score` in the dataset and apply `scoreband.` only where
> you want the label — on `PROC PRINT` for a banded view and on `PROC FREQ` for
> band counts. The format never alters the stored value.

---

## Vintage / cohort analysis

Group loans by their origination month (the "vintage") using `intnx` to snap the
origination date to the first of the month, then aggregate balances per cohort:

```sas
data loans;
  input acct_id $ orig_date : date9. balance;
  cohort     = intnx("month", orig_date, 0, "b");  /* first day of orig. month */
  cohort_yr  = year(orig_date);
  cohort_mo  = month(orig_date);
  format orig_date cohort date9. balance dollar12.2;
  datalines;
A1 05JAN2025 10000
A2 19JAN2025  5000
A3 02FEB2025  7500
A4 27FEB2025  3200
A5 11FEB2025  8800
A6 08MAR2025  6000
;
run;

proc means data=loans n mean min max;
  class cohort;                          /* one block per vintage month */
  var balance;
run;

proc sql;
  create table vintage as
    select cohort_yr, cohort_mo,
           count(*)     as n_loans,
           sum(balance) as orig_balance,
           avg(balance) as avg_balance
    from loans
    group by cohort_yr, cohort_mo
    order by cohort_yr, cohort_mo;
quit;

proc print data=vintage noobs;
  format orig_balance avg_balance dollar12.2;
run;
```

> `intnx("month", d, 0, "b")` returns the first day of the month containing `d`,
> a stable cohort key. Use `PROC SQL` (with `sum`) for cohort totals; `PROC MEANS`
> reports `n`/`mean`/`min`/`max` per cohort.

---

## Account-to-activity left join

Keep every account and attach recent activity where it exists. Sort both inputs
by the key, `merge ... by`, and use `in=` flags to control the join:

```sas
data accounts;
  input acct_id $ owner $ status $;
  datalines;
A100 Alice  open
A101 Bob    open
A102 Carmen closed
A103 Dan    open
;
run;

data activity;
  input acct_id $ last_txn_amt;
  datalines;
A100 1200.00
A101  -15.00
A102  500.00
A104  999.00
;
run;

proc sort data=accounts out=a; by acct_id; run;
proc sort data=activity out=t; by acct_id; run;

data acct_activity;
  merge a(in=ina) t(in=int);
  by acct_id;
  if ina;                         /* keep all accounts; drop activity-only (A104) */
  has_activity = int;             /* 1 if activity matched, else 0 */
  format last_txn_amt dollar10.2;
run;

proc print data=acct_activity noobs; run;
```

> `if ina;` is the subsetting `if` that makes this a left join: rows present only
> in `activity` (like the orphan A104) are dropped, while accounts with no
> activity (A103) survive with a missing `last_txn_amt` and `has_activity=0`.

---

## Running balance per account

Compute a running balance within each account using a BY group and the sum
statement, resetting at the start of each account:

```sas
data ledger;
  input acct_id $ seq amount;
  datalines;
A100 1  1000.00
A100 2  -250.00
A100 3   500.00
A100 4  -120.00
A101 1   300.00
A101 2  -300.00
A101 3    75.00
;
run;

proc sort data=ledger out=l; by acct_id seq; run;

data running;
  set l;
  by acct_id;
  if first.acct_id then balance = 0;    /* reset at each account */
  balance + amount;                     /* sum statement: running total */
  format amount balance dollar10.2;
run;

proc print data=running noobs; run;
```

> The sum statement `balance + amount;` auto-retains across rows and treats
> missing as 0. `first.acct_id` resets it so each account's running balance starts
> fresh. To emit only the closing balance per account, guard `output` with
> `if last.acct_id;`.

---

## Portfolio rollup with PROC SQL

Roll a loan book up by product with totals, averages, a delinquent-balance
share, and a `having` filter to drop tiny segments:

```sas
data loans;
  input acct_id $ product $ region $ balance status $;
  datalines;
L1 auto     East   18000 current
L2 auto     East   22000 current
L3 mortgage West  250000 current
L4 mortgage West  180000 delinquent
L5 card     East    4200 delinquent
L6 card     West    1500 current
L7 auto     West   15000 current
L8 mortgage East  320000 current
;
run;

proc sql;
  create table portfolio as
    select product,
           count(*)                                                   as n_loans,
           sum(balance)                                               as total_bal,
           avg(balance)                                               as avg_bal,
           sum(case when status="delinquent" then balance else 0 end) as delq_bal,
           sum(case when status="delinquent" then balance else 0 end)
             / sum(balance)                                           as delq_rate
    from loans
    group by product
    having sum(balance) > 10000          /* drop immaterial segments (card) */
    order by total_bal desc;
quit;

proc print data=portfolio noobs;
  format total_bal avg_bal delq_bal dollar14.2 delq_rate percent8.1;
run;
```

> `having` filters on the aggregate after grouping, so the small `card` segment
> drops out. The `delq_rate` is a ratio of two aggregates, formatted with
> `percent8.1` for the report.

---

## Loss given default and loss-rate reporting

Compute loss given default (LGD) and portfolio loss rate per segment, then a
portfolio total — all displayed with dollar/percent formats:

```sas
data exposures;
  input segment $ ead defaulted recovered;
  loss      = defaulted - recovered;     /* net credit loss              */
  lgd       = loss / defaulted;          /* loss given default           */
  loss_rate = loss / ead;                /* loss as a share of exposure  */
  datalines;
Prime     1000000 40000 28000
NearPrime  600000 90000 45000
SubPrime   300000 75000 22500
;
run;

proc print data=exposures noobs label;
  label ead="Exposure" loss="Loss" lgd="LGD" loss_rate="Loss rate";
  var segment ead defaulted loss lgd loss_rate;
  format ead defaulted loss dollar14.0 lgd loss_rate percent8.1;
run;

proc sql;
  create table totals as
    select sum(ead)            as ead,
           sum(loss)           as loss,
           sum(loss)/sum(ead)  as portfolio_loss_rate
    from exposures;
quit;

proc print data=totals noobs;
  format ead loss dollar14.0 portfolio_loss_rate percent8.2;
run;
```

> `dollar14.0` and `percent8.1`/`percent8.2` turn raw ratios into a board-ready
> report. The `totals` step aggregates the segment-level losses into a single
> portfolio loss rate.

---

## Reconciliation and data-quality gating

Before trusting an incoming extract, gate it with `PROC PROOF` — an ASS value-add
for declarative validation. Assert key uniqueness, non-null balances, an allowed
status domain, a non-negative balance rule, and referential integrity from
transactions back to accounts. Failing assertions populate an `out=` violations
dataset and make the process exit non-zero:

```sas
data accounts;
  input acct_id $ status $ balance;
  datalines;
A100 open    5200.00
A101 open    1830.50
A102 closed     0.00
A103 frozen   410.25
A100 open     999.00
;
run;

data txns;
  input txn_id $ acct_id $ amount;
  datalines;
T1 A100  -42.50
T2 A101  120.00
T3 A999  -10.00
;
run;

proc proof data=accounts out=acct_bad severity=error;
  require acct_id status balance;                 /* columns must exist        */
  type    acct_id=char balance=num;               /* declared kinds must match */
  notnull acct_id balance;                        /* values present            */
  unique  acct_id;                                /* A100 is duplicated         */
  values  status in ("open" "closed");            /* "frozen" is out of domain  */
  range   balance >= 0;                           /* no negative balances       */
run;

proc proof data=txns out=txn_bad severity=error;
  unique txn_id;
  key acct_id references accounts(acct_id);        /* T3 -> A999 is an orphan    */
run;

proc print data=acct_bad; run;
proc print data=txn_bad;  run;
```

Representative report and violations:

```text
PROC PROOF — WORK.ACCOUNTS (5 obs)
  Assertion                                Result Violations/Checked
  unique acct_id                           FAIL   2/5
  values status in (open closed)           FAIL   1/5
  ...
ERROR: PROC PROOF: 2 error-level assertion(s) failed on WORK.ACCOUNTS.

PROC PROOF — WORK.TXNS (3 obs)
  key acct_id references accounts(acct_id) FAIL   1/3
ERROR: PROC PROOF: 1 error-level assertion(s) failed on WORK.TXNS.
```

> The `out=` dataset gets one row per (source row × failed assertion), annotated
> with `_rule_` and `_obs_`, so violations are trivially filterable. Error-level
> failures log `ERROR` and set a non-zero CLI exit code — ideal for gating a
> pipeline — without halting the rest of the program. See
> [`proofing.md`](proofing.md) for the full assertion catalog.

---

## Parameterizing a delinquency rollup with macros

Compute the same delinquent-balance rate broken out by any dimension, by
parameterizing the grouping variable:

```sas
data loans;
  input acct_id $ region $ product $ balance dpd;
  datalines;
L1 East auto       18000  0
L2 East auto       22000 35
L3 West mortgage  250000  0
L4 West mortgage  180000 95
L5 East card        4200 65
L6 West card        1500  0
;
run;

%macro delq_by(dim);
  proc sql;
    create table delq_&dim as
      select &dim,
             sum(balance)                                   as total_bal,
             sum(case when dpd>=30 then balance else 0 end) as delq_bal,
             sum(case when dpd>=30 then balance else 0 end)
               / sum(balance)                               as delq_rate
      from loans
      group by &dim
      order by delq_rate desc;
  quit;
  proc print data=delq_&dim noobs;
    format total_bal delq_bal dollar14.0 delq_rate percent8.1;
  run;
%mend delq_by;

%delq_by(region)
%delq_by(product)
```

> One macro definition yields a region rollup and a product rollup. `&dim` is
> substituted into the `select`, `group by`, and the output table name
> (`delq_region`, `delq_product`) before parsing.

---

## Reading from a banking database

Banking data usually lives in a database. Bind a libref to your warehouse, then
read tables as datasets — the rest of the recipes above work unchanged on the
result. This example is **illustrative** (it needs a live database and real
credentials):

```sas
libname bank postgres "host=db.internal dbname=core user=analyst";

data work.recent;
  set bank.loans(where=(status="open"));   /* read + filter server-side */
run;

proc sql;
  create table by_product as
    select product, sum(balance) as total_bal
    from bank.loans
    group by product;
quit;
```

> Connection strings and write-back semantics for Postgres, SQL Server, Oracle,
> SQLite, and DB2 are in [`databases.md`](databases.md). A DATA step or PROC can
> also write a table back (`data bank.summary; set work.by_product; run;`), and
> `proc append base=bank.fact data=work.delta;` does an in-place INSERT.

---

For declarative data-quality validation (the gating recipe above), see
[`proofing.md`](proofing.md). For reading and writing external databases
— common in banking pipelines that source from Postgres, Oracle, or DB2 — see
[`databases.md`](databases.md).

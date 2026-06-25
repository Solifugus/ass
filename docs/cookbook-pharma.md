# ASS Cookbook — Pharmaceutical / Clinical Trials

Task-oriented recipes for clinical-trial and submission work with **ASS —
Analyst's Statistical Suite**. Each recipe is a complete, runnable program built
around the kinds of datasets analysts work with every day: subject-level
demographics, adverse events, lab results, and vital signs (the public CDISC
SDTM/ADaM domain concepts — DM, AE, LB, VS — at a generic level). The data here
is small and synthetic; swap in your own dataset names and paths.

New to ASS? Read the [**Tutorial**](tutorial.md) first, then the general
[**Cookbook**](cookbook.md). For the full set of statements, functions, formats,
and options each recipe uses, see the [**Reference**](reference.md).

- [Demographics and per-arm counts](#demographics-and-per-arm-counts)
- [Flagging lab values outside the normal range](#flagging-lab-values-outside-the-normal-range)
- [Adverse-event rates by treatment arm](#adverse-event-rates-by-treatment-arm)
- [Study day and visit windows](#study-day-and-visit-windows)
- [Attaching treatment arm to a child domain](#attaching-treatment-arm-to-a-child-domain)
- [Change from baseline](#change-from-baseline)
- [Last and worst value per subject](#last-and-worst-value-per-subject)
- [Vital-signs summary by arm](#vital-signs-summary-by-arm)
- [A patient listing](#a-patient-listing)
- [Validating submission data with PROC PROOF](#validating-submission-data-with-proc-proof)

---

## Demographics and per-arm counts

Build a subject-level demographics table and count subjects per treatment arm:

```sas
data dm;
  input subjid $ arm $ sex $ age birthdt : date9.;
  format birthdt date9.;
  datalines;
101 PLACEBO M 54 12FEB1969
102 DRUG    F 61 03MAR1962
103 DRUG    M 47 21JUL1976
104 PLACEBO F 39 09NOV1984
105 DRUG    F 58 28APR1965
106 PLACEBO M 66 15JAN1957
;
run;

proc freq data=dm;
  tables arm;          /* one row per arm: count, percent, cumulative */
  tables arm*sex;      /* arm by sex cross-tabulation                 */
run;
```

The one-way table gives the per-arm denominator you cite throughout an analysis:

```
arm      Frequency  Percent  CumFreq  CumPercent
DRUG             3     50.0        3        50.0
PLACEBO          3     50.0        6       100.0
```

> List input with the `: date9.` modifier reads `12FEB1969` into a SAS date; the
> matching `format ... date9.` shows it back in the same form. The cross-tab
> `arm*sex` reports frequency, percent, row percent, and column percent in each
> cell.

---

## Flagging lab values outside the normal range

Derive an abnormal-range indicator (`LOW`/`NORM`/`HIGH`) by comparing each result
to its per-record reference limits:

```sas
data lb;
  input subjid $ visit $ param $ aval lownorm highnorm;
  datalines;
101 WEEK1 ALT  22  7 56
101 WEEK1 GLUC 145 70 99
102 WEEK1 ALT  70  7 56
102 WEEK1 GLUC 88  70 99
103 WEEK1 ALT  41  7 56
103 WEEK1 GLUC 60  70 99
;
run;

data lb_flag;
  set lb;
  length anrind $4;
  if aval < lownorm      then anrind = "LOW";
  else if aval > highnorm then anrind = "HIGH";
  else                        anrind = "NORM";
  abnfl = (anrind ne "NORM");          /* 1 = abnormal, 0 = normal */
run;

proc freq data=lb_flag;
  tables param*anrind;                 /* abnormality counts per analyte */
run;
```

> `length anrind $4;` fixes the flag as a 4-character variable before it is first
> assigned. `abnfl` uses the fact that a comparison evaluates to 1 (true) or 0
> (false), giving a ready-made 0/1 numeric flag.

---

## Adverse-event rates by treatment arm

Cross-tabulate adverse events by arm and test for association with a chi-square:

```sas
data ae;
  input subjid $ arm $ aedecod $ aesev $;
  datalines;
101 PLACEBO HEADACHE MILD
102 DRUG    NAUSEA   MODERATE
102 DRUG    HEADACHE MILD
103 DRUG    NAUSEA   SEVERE
104 PLACEBO HEADACHE MILD
105 DRUG    RASH     MODERATE
105 DRUG    NAUSEA   MILD
106 PLACEBO RASH     MILD
106 PLACEBO HEADACHE MODERATE
;
run;

proc freq data=ae;
  tables arm*aedecod / chisq;
run;
```

The `/ chisq` option adds the Pearson chi-square below the table:

```
Chi-Square  DF=2  Value=3.9375  Prob=0.1396
```

> This counts AE *records* per arm. To count distinct subjects with a given event
> instead, first reduce to one row per subject-event with
> `proc sort nodupkey; by subjid aedecod;` and then run the `proc freq`.

---

## Study day and visit windows

Compute the SAS-style study day (no day 0; days before first dose are negative)
and a visit window from the reference start date, using date functions:

```sas
data visits;
  input subjid $ visit $ rfstdtc : date9. visitdt : date9.;
  format rfstdtc visitdt date9.;
  datalines;
101 BASELINE 01MAR2024 01MAR2024
101 WEEK2    01MAR2024 16MAR2024
101 WEEK4    01MAR2024 28MAR2024
102 BASELINE 10MAR2024 10MAR2024
102 WEEK2    10MAR2024 25MAR2024
;
run;

data sd;
  set visits;
  if visitdt >= rfstdtc then study_day = visitdt - rfstdtc + 1;
  else study_day = visitdt - rfstdtc;
  weeks      = intck("week", rfstdtc, visitdt);   /* whole weeks elapsed */
  window_end = intnx("week", rfstdtc, weeks, "e");/* end of that week    */
  format window_end date9.;
run;

proc print data=sd noobs;
  var subjid visit rfstdtc visitdt study_day weeks window_end;
run;
```

```
subjid  visit     rfstdtc    visitdt    study_day  weeks  window_end
101     BASELINE  01MAR2024  01MAR2024          1      0   02MAR2024
101     WEEK2     01MAR2024  16MAR2024         16      2   16MAR2024
101     WEEK4     01MAR2024  28MAR2024         28      4   30MAR2024
102     BASELINE  10MAR2024  10MAR2024          1      0   16MAR2024
102     WEEK2     10MAR2024  25MAR2024         16      2   30MAR2024
```

> SAS dates are integers (days since 1960-01-01), so `visitdt - rfstdtc` is just
> arithmetic. `intck("week", ...)` counts interval boundaries crossed; `intnx`
> advances a date by a number of intervals — here `"e"` lands on the *end* of the
> target week. The base intervals `day`/`week`/`month`/`qtr`/`year` are supported;
> `mdy(m,d,y)` builds a date from numeric parts when you need it.

---

## Attaching treatment arm to a child domain

Carry subject-level variables (arm, from DM) onto every row of a child domain
(AE) with a one-to-many `merge ... by`, and find subjects with no events:

```sas
data dm;
  input subjid $ arm $ sex $ age;
  datalines;
101 PLACEBO M 54
102 DRUG    F 61
103 DRUG    M 47
104 PLACEBO F 39
;
run;

data ae;
  input subjid $ aedecod $;
  datalines;
102 NAUSEA
102 HEADACHE
103 RASH
105 NAUSEA
;
run;

proc sort data=dm out=dm_s; by subjid; run;
proc sort data=ae out=ae_s; by subjid; run;

/* Arm carried onto each AE row; keep only rows that have an event. */
data ae_arm;
  merge dm_s(in=ind) ae_s(in=ina);
  by subjid;
  if ina;
run;

/* Enrolled subjects (in DM) with no adverse event at all. */
data no_ae;
  merge dm_s(in=ind) ae_s(in=ina);
  by subjid;
  if ind and not ina;
  keep subjid arm;
run;

proc print data=ae_arm noobs; var subjid arm aedecod; run;
proc print data=no_ae  noobs; run;
```

> Both inputs must be sorted by the BY key first. In a one-to-many merge the DM
> values are retained across each matching AE row. The `in=` flags name where a
> row came from: `if ina` keeps every AE; `if ind and not ina` isolates enrolled
> subjects who never reported an event. (Note `in=` is true only on the *first*
> row of a BY group from that source, so use it to gate provenance, not to count
> replicated rows.)

---

## Change from baseline

Compute change and percent change from each subject's first (baseline) visit
using BY-group `first.` and `retain`:

```sas
data vs;
  input subjid $ visitnum aval;
  datalines;
101 1 120
101 2 128
101 3 118
102 1 140
102 2 135
102 3 130
;
run;

proc sort data=vs out=vs_s; by subjid visitnum; run;

data cfb;
  set vs_s;
  by subjid;
  retain base;
  if first.subjid then base = aval;        /* baseline = first visit */
  chg = aval - base;                       /* change from baseline   */
  if base ne 0 then pchg = 100 * chg / base;
  format pchg 6.1;
run;

proc print data=cfb noobs;
  var subjid visitnum aval base chg pchg;
run;
```

```
subjid  visitnum  aval  base  chg  pchg
101            1   120   120    0   0.0
101            2   128   120    8   6.7
101            3   118   120   -2  -1.7
102            1   140   140    0   0.0
102            2   135   140   -5  -3.6
102            3   130   140  -10  -7.1
```

> `retain base;` keeps `base` from resetting to missing each iteration; setting it
> only `if first.subjid` freezes the baseline for the whole subject. Every later
> row then subtracts that retained baseline.

---

## Last and worst value per subject

Reduce a longitudinal domain to one row per subject — either the last scheduled
visit or the worst (here, highest) value — by sorting so the wanted row is last
and keeping `if last.`:

```sas
data lb;
  input subjid $ visitnum aval;
  datalines;
101 1 22
101 2 41
101 3 30
102 1 70
102 2 65
102 3 88
103 1 18
103 2 19
;
run;

/* Last scheduled visit per subject. */
proc sort data=lb out=lb_v; by subjid visitnum; run;
data last_visit;
  set lb_v;
  by subjid;
  if last.subjid;
run;

/* Worst (highest) value per subject. */
proc sort data=lb out=lb_w; by subjid aval; run;
data worst_high;
  set lb_w(rename=(aval=worst_aval));
  by subjid;
  if last.subjid;
run;

proc print data=last_visit noobs; var subjid visitnum aval; run;
proc print data=worst_high noobs; var subjid worst_aval;    run;
```

```
subjid  visitnum  aval      subjid  worst_aval
101            3    30      101             41
102            3    88      102             88
103            2    19      103             19
```

> The trick is the sort key. Sorting by `visitnum` puts the final visit last;
> sorting by `aval` puts the maximum last. `if last.subjid;` then keeps exactly the
> last row of each BY group. For the lowest value instead, add `descending aval`
> to the sort. Rename through the `set` dataset option (`rename=(aval=worst_aval)`).

---

## Vital-signs summary by arm

Descriptive statistics for vital signs, grouped by treatment arm:

```sas
data vs;
  input subjid $ arm $ sbp dbp;
  datalines;
101 PLACEBO 132 84
102 DRUG    128 80
103 DRUG    140 90
104 PLACEBO 122 78
105 DRUG    135 85
106 PLACEBO 145 92
;
run;

proc means data=vs n mean stddev min max;
  class arm;
  var sbp dbp;
run;
```

```
arm      Variable  N          Mean        StdDev  Min  Max
DRUG     sbp       3  134.33333333  6.0277137733  128  140
DRUG     dbp       3            85             5   80   90
PLACEBO  sbp       3           133  11.532562595  122  145
PLACEBO  dbp       3  84.666666667  7.0237691686   78   92
```

> `class arm;` produces one summary block per arm without pre-sorting; use
> `by arm;` instead when the data is already sorted and you want separate report
> blocks. `PROC SUMMARY` is the same engine.

---

## A patient listing

A clean subject listing with labels and display formats — including user value
formats for sex and age group:

```sas
proc format;
  value $sexf  "M"="Male" "F"="Female";
  value agegrpf low-17="<18" 18-64="18-64" 65-high="65+";
run;

data dm;
  input subjid $ arm $ sex $ age rfstdtc : date9. wt;
  label subjid  = "Subject ID"
        arm     = "Treatment Arm"
        sex     = "Sex"
        age     = "Age (years)"
        rfstdtc = "First Dose Date"
        wt      = "Weight (kg)";
  format rfstdtc date9. sex $sexf. age agegrpf. wt 6.1;
  datalines;
101 PLACEBO M 54 01MAR2024 78.4
102 DRUG    F 61 03MAR2024 64.0
103 DRUG    M 47 05MAR2024 90.2
104 PLACEBO F 16 07MAR2024 52.5
;
run;

proc print data=dm noobs label;
  var subjid arm sex age rfstdtc wt;
run;
```

```
Subject ID  Treatment Arm  Sex     Age (years)  First Dose Date  Weight (kg)
101         PLACEBO        Male          18-64        01MAR2024         78.4
102         DRUG           Female        18-64        03MAR2024         64.0
103         DRUG           Male          18-64        05MAR2024         90.2
104         PLACEBO        Female          <18        07MAR2024         52.5
```

> `proc print ... label` uses the variable labels as column headers; the attached
> formats render sex as Male/Female, age as a banded group, and the date in
> `date9.` form. Define value formats once with `PROC FORMAT` and reuse them for
> both display (PRINT) and grouping (FREQ/MEANS).

---

## Validating submission data with PROC PROOF

Before handing a domain off, assert its data-quality rules in one place. `PROC
PROOF` (an ASS value-add — see [`proofing.md`](proofing.md)) checks required
columns, value domains, ranges, and referential integrity to a parent domain,
writes the offending rows to an `out=` dataset, and exits non-zero when an
error-level assertion fails — ideal for a submission gate:

```sas
/* Parent domain: enrolled subjects. */
data dm;
  input subjid $ arm $ age;
  datalines;
101 PLACEBO 54
102 DRUG    61
103 DRUG    47
104 PLACEBO 39
;
run;

/* Child domain: adverse events, with a couple of seeded problems. */
data ae;
  input subjid $ aedecod $ aesev $ aeser;
  datalines;
101 HEADACHE MILD     0
102 NAUSEA   MODERATE 0
103 RASH     SEVERE   1
109 NAUSEA   MILD     0
104 FATIGUE  HUGE     0
;
run;

proc proof data=ae out=ae_bad severity=error;
  require subjid aedecod aesev;                  /* columns must exist        */
  notnull subjid aedecod;                        /* key fields populated      */
  values aesev in ("MILD" "MODERATE" "SEVERE");  /* allowed severity grades   */
  range aeser 0 - 1;                             /* serious flag is 0/1       */
  key subjid references dm(subjid);              /* every AE ties to a subject*/
run;

proc print data=ae_bad noobs; run;
```

The verdict names each failing assertion and the offending observations:

```
PROC PROOF — WORK.AE (5 obs)
  Assertion                                Result Violations/Checked
  require subjid aedecod aesev             PASS   0/1
  notnull subjid aedecod                   PASS   0/5
  values aesev in (MILD MODERATE SEVERE)   FAIL   1/5
  range aeser 0 - 1                        PASS   0/5
  key subjid references dm(subjid)         FAIL   1/5
ERROR: PROC PROOF: 2 error-level assertion(s) failed on WORK.AE.
```

and the `out=` dataset isolates the rows to fix — the bad severity (`HUGE`) and
the orphan subject (`109`, not in DM):

```
subjid  aedecod  aesev  aeser  _rule_                                  _obs_
109     NAUSEA   MILD       0  key subjid references dm(subjid)            4
104     FATIGUE  HUGE       0  values aesev in (MILD MODERATE SEVERE)      5
```

> Each assertion can carry `/ severity=warn` (logs a warning, exit code
> unaffected) or `severity=error` (logs an error, makes the process exit
> non-zero). Use `unique subjid;` on DM to assert one row per subject, and chain a
> `PROC PROOF` per domain in a CI job so a bad extract fails the build. The full
> assertion set — `require`, `type`, `notnull`, `values`, `range`, `unique`,
> `key … references`, and free-form `rule "label": <expr>` — is documented in
> [`proofing.md`](proofing.md).

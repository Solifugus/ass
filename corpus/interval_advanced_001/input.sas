/* Advanced intck/intnx interval forms: multipliers (MONTH2), shifts (WEEK.2),
   the SEMIYEAR interval, datetime (DT-prefixed) intervals, and sub-day intervals
   (HOUR/MINUTE). All values derive from d = '15JAN2020'd (SAS day 21929, a
   Wednesday) and a 14:30:00 time value. */
data ivals;
  d = '15JAN2020'd;                       /* 21929 */

  /* Multiplier: MONTH2 = bimonthly runs Jan/Mar/May/... */
  bimo   = intnx('month2', d, 1);         /* 01MAR2020 -> 21975 */
  bimock = intck('month2', d, '01MAY2020'd); /* 2 bimonthly boundaries */

  /* Shift: WEEK starts Sunday; WEEK.2 starts Monday. */
  sunwk = intnx('week',   d, 0);          /* Sun 12JAN2020 -> 21926 */
  monwk = intnx('week.2', d, 0);          /* Mon 13JAN2020 -> 21927 */

  /* SEMIYEAR: half-year runs Jan/Jul. */
  semiyr = intnx('semiyear', d, 1);       /* 01JUL2020 -> 22097 */

  /* Datetime (DT-prefixed) interval over a datetime value. */
  dt      = dhms(d, 0, 0, 0);             /* 21929 * 86400 */
  dtnext  = intnx('dtday', dt, 3);        /* +3 days */
  dtdaynum = datepart(dtnext);            /* 21932 */
  dtck    = intck('dtday', dt, dtnext);   /* 3 */

  /* Sub-day intervals on a time value. */
  t    = hms(14, 30, 0);                  /* 52200 */
  hr   = intnx('hour', t, 2);             /* 16:00:00 -> 57600 */
  hrck = intck('hour', t, hms(16, 30, 0));/* 2 hour boundaries */
  mn   = intnx('minute', 90, 1);          /* 00:02:00 -> 120 */

  keep bimo bimock sunwk monwk semiyr dtdaynum dtck hr hrck mn;
run;

proc print data=ivals; run;

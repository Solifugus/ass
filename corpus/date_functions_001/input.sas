/* Date/time functions over SAS date/datetime encodings (days since 1960-01-01,
   seconds since 1960-01-01 for datetime, seconds since midnight for time). */
data dt;
  d = '15JAN2020'd;                  /* 21929 */
  y  = year(d);                      /* 2020 */
  m  = month(d);                     /* 1 */
  dd = day(d);                       /* 15 */
  q  = qtr(d);                       /* 1 */
  wd = weekday(d);                   /* 15JAN2020 is a Wednesday -> 4 */
  built  = mdy(2, 29, 2020);         /* 29FEB2020 -> 21974 */
  nmonths = intck('month', d, '20MAR2020'd);  /* 2 */
  nextmo = intnx('month', d, 1);     /* 01FEB2020 -> 21946 */
  eom    = intnx('month', d, 0, 'e');/* 31JAN2020 -> 21945 */
  stamp  = dhms(d, 1, 0, 0);         /* datetime: 21929*86400 + 3600 */
  dp = datepart(stamp);              /* 21929 */
  tp = timepart(stamp);              /* 3600 */
  t  = hms(1, 30, 0);                /* 5400 */
  keep y m dd q wd built nmonths nextmo eom dp tp t;
run;

proc print data=dt; run;

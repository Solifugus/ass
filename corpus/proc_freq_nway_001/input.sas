/* PROC FREQ n-way list table and two-way chi-square.

   `tables region*product*size / list` produces one row per distinct combination
   with Frequency/Percent/cumulative columns. `tables region*product / chisq`
   appends the Pearson chi-square statistic, its DF, and p-value. FREQ prints to
   the listing (no output dataset), so values are asserted by the Go unit tests
   (TestFreqNWayList, TestChiSquareStat). */
data d;
  input region $ product $ size $;
  datalines;
N A S
N A L
N B S
S A S
S B L
S B L
;
run;

proc freq data=d;
  tables region*product*size / list;
run;

proc freq data=d;
  tables region*product / chisq nocol norow;
run;

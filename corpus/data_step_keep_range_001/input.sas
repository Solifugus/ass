/* Numbered variable-list ranges expand in keep=/drop=.

   `keep id x1-x3;` (statement form) and `drop=x2-x3` (dataset-option form)
   expand to x1,x2,x3 / x2,x3. The kept output has id,x1,x2,x3; the dropped
   output has id,x1 (x2 and x3 removed). */
data wide;
  input id x1 x2 x3 x4;
  datalines;
1 10 20 30 40
2 11 21 31 41
;
run;

data narrow;
  set wide;
  keep id x1-x3;
run;

data fewer;
  set wide(drop=x2-x3);
run;

proc print data=narrow; run;
proc print data=fewer; run;

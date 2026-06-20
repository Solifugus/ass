data sales;
  input region $ amt;
  datalines;
east 100
west 200
;
run;

data q2;
  input region $ amt;
  datalines;
east 50
north 75
;
run;

/* BASE=all does not exist yet: PROC APPEND creates it from SALES. */
proc append base=all data=sales;
run;

/* Append Q2 in place: ALL now holds all four observations, in order. */
proc append base=all data=q2;
run;

proc print data=all;
run;

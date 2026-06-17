data running;
  retain total 0;
  input sale;
  total + sale;
  datalines;
10
20
30
;
run;

proc print data=running;
run;

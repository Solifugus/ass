data sales;
  input region $ amount;
  datalines;
east 10
east 20
west 30
west 40
;
run;

data totals;
  set sales;
  by region;
  retain total;
  if first.region then total = 0;
  total + amount;
  if last.region then output;
  keep region total;
run;

proc print data=totals;
run;

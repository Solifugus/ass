data sales;
  input item $ qty price;
  total = qty * price;
  datalines;
apple 3 2
bread 2 4
milk 1 5
;
run;

proc print data=sales;
  var item total;
run;

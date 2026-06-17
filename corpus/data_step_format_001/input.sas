data sales;
  input item $ amount;
  format amount dollar10.2;
  datalines;
apple 1234.5
bread 56789
;
run;

proc print data=sales;
run;

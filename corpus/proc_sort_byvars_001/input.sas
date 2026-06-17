data people;
  input name $ age;
  datalines;
John 25
Mary 30
Tim 12
Ann 17
;
run;

proc sort data=people;
  by age;
run;

proc print data=people;
run;

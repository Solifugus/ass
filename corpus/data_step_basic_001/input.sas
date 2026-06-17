data people;
  input name $ age;
  datalines;
John 25
Mary 30
;
run;

proc print data=people;
run;

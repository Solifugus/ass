data people;
  input name $ age;
  datalines;
John 25
Mary 30
Tim 12
;
run;

data numbered;
  set people;
  row = _N_;
run;

proc print data=numbered;
run;

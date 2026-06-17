data people;
  input name $ age;
  datalines;
John 25
Mary 30
Tim 12
Ann 17
;
run;

data adults;
  set people;
  if age >= 18;
run;

proc print data=adults;
run;

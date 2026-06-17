%let cutoff = 18;

data people;
  input name $ age;
  datalines;
John 25
Mary 30
Tim 12
;
run;

data adults;
  set people;
  if age >= &cutoff;
run;

proc print data=adults;
run;

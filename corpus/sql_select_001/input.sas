data people;
  input name $ age;
  datalines;
John 25
Mary 30
Tim 12
;
run;

proc sql;
  select name, age
  from people
  where age >= 18;
quit;

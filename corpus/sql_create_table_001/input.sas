data people;
  input name $ age;
  datalines;
John 25
Mary 30
Tim 12
;
run;

proc sql;
  create table adults as
  select *
  from people
  where age >= 18;
quit;

proc print data=adults;
run;

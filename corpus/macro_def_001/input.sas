%macro show(ds);
  proc print data=&ds;
  run;
%mend show;

data people;
  input name $ age;
  datalines;
John 25
Mary 30
;
run;

%show(people)

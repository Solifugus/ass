data visits;
  input id day $;
  datalines;
1 mon
1 tue
2 mon
2 wed
3 fri
;
run;

proc sort data=visits out=unique nodupkey;
  by id;
run;

proc print data=unique;
run;

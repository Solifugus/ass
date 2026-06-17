data scores;
  input grp $ score;
  datalines;
a 10
a 20
a 30
b 100
b 200
;
run;

proc means data=scores;
  class grp;
  var score;
run;

data orig;
  input x y z;
  datalines;
1 2 3
4 5 6
;
run;

data renamed;
  set orig;
  keep x y;
  rename x=alpha y=beta;
run;

proc print data=renamed;
run;

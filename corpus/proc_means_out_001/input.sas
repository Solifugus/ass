data s;
  input grp $ x;
  datalines;
a 10
a 20
a 30
b 100
b 200
;
run;

proc means data=s;
  class grp;
  var x;
  output out=stats n=nx mean=mx sum=sx min=mn max=mxx;
run;

proc print data=stats;
run;

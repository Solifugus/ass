data regions;
  input region $;
  datalines;
east
west
;
run;

data orders;
  input id qty region $;
  datalines;
1 10 east
2 20 south
3 30 west
;
run;

proc proof data=orders out=bad;
  type id=num region=char;
  key region references regions(region);
run;

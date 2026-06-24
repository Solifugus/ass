data orders;
  input id qty region $;
  datalines;
1 10 east
2 0 west
2 50 east
;
run;

proc proof data=orders out=bad;
  notnull qty;
  range qty 1 - 100;
  values region in ("east" "west");
  unique id;
run;

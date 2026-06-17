data xy;
  input x y;
  datalines;
1 1
2 3
3 2
4 5
5 4
;
run;

proc reg data=xy;
  model y = x;
run;

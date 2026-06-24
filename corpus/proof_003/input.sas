data dim;
  input region $ yr;
  datalines;
east 2024
west 2024
;
run;

data f;
  input region $ yr amt num den;
  datalines;
east 2024 50 4 2
west 2025 -5 9 3
;
run;

proc proof data=f out=bad;
  range amt >= 0;
  rule "ratio ok": num / den < 5;
  key region yr references dim(region yr);
run;

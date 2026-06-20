/* Trailing line-hold modifiers on INPUT.

   `@@` (double trailing-at) holds the physical line across iterations, so several
   observations are read from one line — and it keeps going across line breaks
   until the data is exhausted. Here three (x,y) pairs are read from two lines.

   `@` (single trailing-at) holds the line within one iteration: a control value
   is read, then the rest of the line is read conditionally. Each physical line is
   one observation. */
data pairs;
  input x y @@;
  datalines;
1 2 3 4
5 6
;
run;

data classified;
  input kind $ @;
  if kind = 'N' then input value;
  else input label $;
  datalines;
N 42
T hello
N 7
;
run;

proc print data=pairs; run;
proc print data=classified; run;

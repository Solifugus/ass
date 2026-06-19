/* Column input reads each variable from a fixed character range, so a value may
   contain blanks that delimited list input would split on. Here NAME spans
   columns 5-14 and keeps the embedded space in "Mary Ann"; ID (1-3) and AGE
   (16-17) read from their own columns. */
data people;
  input id 1-3 name $ 5-14 age 16-17;
  datalines;
  1 Mary Ann   42
 12 Bob         7
;
run;

proc print data=people;
run;

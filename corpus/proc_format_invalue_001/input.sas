/* PROC FORMAT INVALUE defines user informats that map input to values.

   - grade. (numeric result) maps letter grades to grade points, other=0.
   - $resp. (character result) maps Y/N to Yes/No.
   - band. (numeric ranges) maps 1-10 -> 1, 11-20 -> 2, other -> 9.
   INPUT reads each field through its user informat. */
proc format;
  invalue grade 'A'=4 'B'=3 'C'=2 other=0;
  invalue $resp 'Y'='Yes' 'N'='No';
  invalue band 1-10=1 11-20=2 other=9;
run;

data scores;
  input letter grade. r $resp. n band.;
  datalines;
A Y 5
B N 15
Z Y 99
;
run;

proc print data=scores; run;

/* Time/datetime literals and informats.

   - `'14:30:00't` is a SAS time = seconds since midnight = 52200.
   - `'01JAN1960:00:00:01'dt` is a SAS datetime = seconds since 1960-01-01 = 1.
   - The TIME and DATETIME informats read the same text forms back to the same
     numeric values, so `lits` (from literals) and `read` (from informats) must
     agree. */
data lits;
  t  = '14:30:00't;
  dt = '01JAN1960:00:00:01'dt;
  output;
run;

data read;
  input t time8. dt datetime19.;
  datalines;
14:30:00 01JAN1960:00:00:01
;
run;

proc print data=lits; run;
proc print data=read; run;

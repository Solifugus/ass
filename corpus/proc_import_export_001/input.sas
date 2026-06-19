/* Round-trip a dataset through CSV with PROC EXPORT then PROC IMPORT.
   EXPORT writes a header row of column names and DSD-quotes the embedded comma
   in "Smith, John"; IMPORT reads the header for column names (GETNAMES=YES,
   the default) and sniffs the numeric SALARY column from the data rows. The
   re-imported dataset must match the original values. @TMP@ is a per-run temp
   directory supplied by the harness. */
data staff;
  name = "Smith, John"; salary = 52000; output;
  name = "Mary";        salary = 48500; output;
run;

proc export data=staff outfile="@TMP@/staff.csv" dbms=csv replace;
run;

proc import datafile="@TMP@/staff.csv" out=back dbms=csv replace;
  getnames=yes;
run;

proc print data=back;
run;

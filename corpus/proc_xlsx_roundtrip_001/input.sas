/* PROC EXPORT/IMPORT round-trip through an .xlsx workbook.

   EXPORT writes the dataset to a single worksheet (header row + data; numeric
   cells as numbers, character cells as strings); IMPORT reads it back and sniffs
   column types. The re-imported `back` dataset must equal the original values.
   @TMP@ is a per-run temp directory supplied by the harness. */
data sales;
  input region $ amount;
  datalines;
North 100
South 250
East 75
;
run;

proc export data=sales outfile="@TMP@/sales.xlsx" dbms=xlsx replace; run;

proc import datafile="@TMP@/sales.xlsx" out=back dbms=xlsx replace; run;

proc print data=back; run;

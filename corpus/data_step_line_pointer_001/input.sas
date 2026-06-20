/* `#n` line pointers on INPUT and PUT.

   On INPUT, `#n` reads one observation across several physical lines: `#1` reads
   from the first line of the record, `#2` from the second (the column pointer
   resets to column 1 at each `#n`). Here each person is two lines — name/age on
   line 1, city/zip on line 2 — so two records produce two observations.

   On PUT, `#n` writes one observation as several physical lines, mirroring the
   read. The data _null_ step rewrites each observation back to a two-line record.
   The reloaded dataset must equal the original, value for value. */
data people;
  input #1 name $ age #2 city $ zip;
  datalines;
John 25
Boston 2134
Mary 30
Austin 78701
;
run;

data _null_;
  set people;
  file "@TMP@/people_out.txt";
  put #1 name age #2 city zip;
run;

data roundtrip;
  infile "@TMP@/people_out.txt";
  input #1 name $ age #2 city $ zip;
run;

proc print data=people; run;
proc print data=roundtrip; run;

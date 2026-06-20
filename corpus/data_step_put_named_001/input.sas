/* Named PUT output (`put var=;`) writes each value as name=value.

   `put id=;` emits "id=1" / "id=2"; reading the file back with an `=`-delimited
   INFILE recovers the label and value, so the round-trip value-verifies the
   name=value rendering. `put _all_;` is exercised to the log to confirm it
   parses and executes (its output is not value-checked here). */
data src;
  input id name $;
  datalines;
1 Amy
2 Bob
;
run;

data _null_;
  set src;
  file "@TMP@/named.txt";
  put id=;
run;

data back;
  infile "@TMP@/named.txt" dlm='=';
  input lbl $ idval;
run;

data _null_;
  set src;
  put _all_;
run;

proc print data=back; run;

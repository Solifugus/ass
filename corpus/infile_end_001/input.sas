/* INFILE END= flag: read an external file and accumulate a running total, emitting
   a single summary row only when the last record has been read. The END= variable
   (here `last`) is temporary and is not written to the output dataset. */
data summary;
  infile "@DIR@/nums.txt" end=last;
  input value;
  total + value;     /* sum statement: running total, retained */
  count + 1;
  if last then output;
run;

proc print data=summary; run;

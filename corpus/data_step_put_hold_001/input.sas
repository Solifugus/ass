/* Trailing line-hold on PUT (output hold).

   `put n @@;` holds the output line across DATA-step iterations, so several
   observations are written to one physical line ("1 2 3"); reading it back with
   the matching `input n @@;` recovers one observation per value.

   `put name @; put age;` uses a single trailing `@` to hold the line within the
   iteration so two PUTs build one line per observation ("Amy 30" / "Bob 40");
   list INPUT reads both values back.

   @TMP@ is a per-run temp directory supplied by the harness. */
data src;
  input n;
  datalines;
1
2
3
;
run;

data _null_;
  set src;
  file "@TMP@/holds.txt";
  put n @@;
run;

data back;
  infile "@TMP@/holds.txt";
  input n @@;
run;

data src2;
  input name $ age;
  datalines;
Amy 30
Bob 40
;
run;

data _null_;
  set src2;
  file "@TMP@/single.txt";
  put name @;
  put age;
run;

data back2;
  infile "@TMP@/single.txt";
  input name $ age;
run;

proc print data=back; run;
proc print data=back2; run;

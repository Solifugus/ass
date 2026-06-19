/* Column/pointer PUT output writes each value into a fixed column span: NAME is
   left-justified in columns 1-10, AGE right-justified in 11-13. The file is then
   read back with the matching column INPUT, so the re-imported dataset must equal
   the original values — verifying both column output and column input.
   @TMP@ is a per-run temp directory supplied by the harness. */
data src;
  name = "Mary Ann"; age = 42; output;
  name = "Bob";      age = 7;  output;
run;

data _null_;
  set src;
  file "@TMP@/fixed.txt";
  put name $ 1-10 age 11-13;
run;

data back;
  infile "@TMP@/fixed.txt";
  input name $ 1-10 age 11-13;
run;

proc print data=back;
run;

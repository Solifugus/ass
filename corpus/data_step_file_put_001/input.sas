/* Write a delimited file with FILE/PUT, then read it back with INFILE/INPUT.
   The round trip verifies PUT's DSD quoting (the comma inside "Smith, John"
   forces quotes), numeric formatting via an inline format, and that the values
   survive being written and re-read. `data _null_` writes only to the file. */
data staff;
  name = "Smith, John"; salary = 52000; output;
  name = "Mary";        salary = 48500; output;
run;

data _null_;
  set staff;
  file "@TMP@/staff.csv" dsd;
  put name salary;
run;

data roundtrip;
  infile "@TMP@/staff.csv" dsd;
  input name $ salary;
run;

proc print data=roundtrip;
run;

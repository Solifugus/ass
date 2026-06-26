data s;
  input region $ product $;
  datalines;
e a
e a
w b
e b
w a
;
run;

proc freq data=s;
  tables region / out=onef;
  tables region*product / out=twof;
run;

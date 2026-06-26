proc format;
  value $reg "e"="East" "w"="West";
run;

data s;
  input region $;
  datalines;
e
e
w
e
w
;
run;

proc freq data=s;
  tables region / out=rf;
  format region $reg.;
run;

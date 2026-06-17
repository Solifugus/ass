data t;
  input id name $ pay : comma8. hired : date9.;
  format pay dollar10.2 hired date9.;
  datalines;
1 Anna 1,234 15JAN2020
2 Bob 56,789 01JUL2021
;
run;

proc print data=t;
run;

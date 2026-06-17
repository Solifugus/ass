data events;
  input label $;
  start = '01JAN2020'd;
  format start date9.;
  datalines;
launch
review
;
run;

proc print data=events;
run;

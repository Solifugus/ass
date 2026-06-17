data pets;
  input kind $;
  datalines;
cat
dog
cat
fish
dog
cat
;
run;

proc freq data=pets;
  tables kind;
run;

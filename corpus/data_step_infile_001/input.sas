data people;
  infile "@DIR@/people.csv" dsd firstobs=2;
  input name $ age city $;
run;

proc print data=people;
run;

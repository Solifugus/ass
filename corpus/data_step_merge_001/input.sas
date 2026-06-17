data names;
  input id name $;
  datalines;
1 John
2 Mary
3 Tim
;
run;

data scores;
  input id score;
  datalines;
1 95
2 85
4 70
;
run;

data matched;
  merge names(in=n) scores(in=s);
  by id;
  if n and s;
run;

proc print data=matched;
run;

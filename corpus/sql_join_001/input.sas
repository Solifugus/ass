data people;
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
3 70
;
run;

proc sql;
  select p.name, s.score
  from people as p, scores as s
  where p.id = s.id;
quit;

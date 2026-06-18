data trial;
  input grp $ score;
  datalines;
A 10
A 12
A 11
B 20
B 23
B 19
C 31
C 29
C 30
;
run;

proc glm data=trial;
  class grp;
  model score = grp;
run;

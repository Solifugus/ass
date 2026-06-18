data people;
  input id age dept $;
  datalines;
1 25 A
2 40 B
3 33 A
4 19 C
5 55 B
;
run;

data adults;
  set people(where=(age >= 30) keep=id age dept rename=(age=years));
run;

proc print data=adults;
run;

proc print data=people(where=(dept='B'));
run;

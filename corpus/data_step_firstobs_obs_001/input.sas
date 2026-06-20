/* FIRSTOBS=/OBS= dataset options select an observation range by position.

   `set people(firstobs=2 obs=4)` reads observations 2..4 (OBS= is the number of
   the last observation processed, not a row count). `where=` is applied after the
   positional selection, so `mid_adult` keeps only ids 2 and 3 (ages >= 30) from
   the 2..4 window. */
data people;
  input id age;
  datalines;
1 25
2 40
3 33
4 19
5 55
;
run;

data mid;
  set people(firstobs=2 obs=4);
run;

data mid_adult;
  set people(firstobs=2 obs=4 where=(age >= 30));
run;

proc print data=mid; run;
proc print data=mid_adult; run;

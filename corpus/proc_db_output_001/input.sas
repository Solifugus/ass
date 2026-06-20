libname db sqlite "@TMP@/procout.db";

data people;
  input id name $ region $ amt;
  datalines;
3 Carol west 50
1 Alice east 100
2 Bob east 200
;
run;

/* PROC SORT writes its OUT= dataset into the database. */
proc sort data=people out=db.sorted;
  by id;
run;

/* PROC SQL materializes an aggregate straight into the database. */
proc sql;
  create table db.totals as
    select region, sum(amt) as total
    from people
    group by region
    order by region;
quit;

/* Read both back from the database for value verification. */
data sorted_back;
  set db.sorted;
run;

data totals_back;
  set db.totals;
run;

proc print data=sorted_back;
run;

proc print data=totals_back;
run;

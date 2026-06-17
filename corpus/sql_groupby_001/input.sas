data sales;
  input region $ amount;
  datalines;
east 100
west 200
east 150
west 50
east 75
;
run;

proc sql;
  select region, sum(amount) as total
  from sales
  group by region
  order by region;
quit;

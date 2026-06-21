/* PROC FREQ groups by a user VALUE format: ages below 30 collapse into "Young",
   30 and above into "Older". The one-way table should show two categories,
   Young (2: ages 22, 25) and Older (3: ages 40, 55, 33). */
proc format;
  value agegrp low-29='Young' 30-high='Older';
run;

data people;
  input age;
  datalines;
22
25
40
55
33
;
run;

proc freq data=people;
  tables age;
  format age agegrp.;
run;

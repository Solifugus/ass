proc format;
  value agegrp
    low - 12 = 'Child'
    13 - 19 = 'Teen'
    20 - high = 'Adult';
  value $sex
    'M' = 'Male'
    'F' = 'Female'
    other = 'Unknown';
run;

data people;
  input name $ age sex $;
  format age agegrp. sex $sex.;
  datalines;
Anna 8 F
Bob 16 M
Cara 45 F
Dan 30 X
;
run;

proc print data=people;
run;

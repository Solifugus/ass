data sales;
  input region $ product $;
  datalines;
North A
North A
North B
North A
North B
South A
South B
South B
South B
South B
;
run;

proc freq data=sales;
  tables region*product;
run;

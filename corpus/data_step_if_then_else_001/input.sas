data graded;
  input name $ score;
  if score >= 90 then grade = 'A';
  else if score >= 80 then grade = 'B';
  else grade = 'C';
  datalines;
John 95
Mary 85
Tim 70
;
run;

proc print data=graded;
run;

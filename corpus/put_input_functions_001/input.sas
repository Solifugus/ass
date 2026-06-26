proc format;
  value scoreband low-599="Poor" 600-749="Fair" 750-high="Good";
run;

data scored;
  input score raw $;
  band   = put(score, scoreband.);
  amount = input(raw, comma8.);
  datalines;
580 1,234
700 2,500
800 9,999
;
run;

proc print data=scored;
run;

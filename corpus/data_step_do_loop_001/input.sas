data squares;
  do i = 1 to 5;
    sq = i * i;
    output;
  end;
run;

proc print data=squares;
run;

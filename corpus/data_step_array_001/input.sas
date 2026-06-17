data doubled;
  array s{3} s1 s2 s3;
  input s1 s2 s3;
  do i = 1 to 3;
    s{i} = s{i} * 2;
  end;
  drop i;
  datalines;
1 2 3
10 20 30
;
run;

proc print data=doubled;
run;

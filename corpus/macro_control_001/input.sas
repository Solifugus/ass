%macro gen(n);
  data nums;
    %do i = 1 %to &n;
      x = &i;
      output;
    %end;
  run;
%mend gen;

%gen(3)

proc print data=nums;
run;

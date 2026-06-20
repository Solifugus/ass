data employees;
  input id name $ age;
  label id = "Employee ID"
        name = "Full Name"
        age = "Age in Years";
  datalines;
1 Alice 30
2 Bob 45
;
run;

data copy;
  set employees;
run;

proc print data=copy label;
  var id name age;
run;

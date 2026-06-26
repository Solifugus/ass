/* PROC FORMAT CNTLOUT=: persist user VALUE formats to a control dataset. The
   dataset uses the standard columns FMTNAME/START/END/LABEL/TYPE/HLO/SEXCL/EEXCL;
   HLO flags L (START is LOW), H (END is HIGH), O (OTHER). Formats sort by name,
   so $REG (stored REG, TYPE C) precedes AGEGRP. */
proc format;
  value agegrp low-12='Child' 13-19='Teen' 20-high='Adult' other='?';
  value $reg 'E'='East' 'W'='West';
run;

proc format cntlout=fmts;
run;

proc print data=fmts; run;

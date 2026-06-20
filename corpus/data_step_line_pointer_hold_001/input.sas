/* Combining a #n line pointer with a trailing @@ hold.

   `input a #2 b @@;` reads a 2-line logical record (a from line 1, b from line
   2) while @@ holds the record group across iterations with per-line cursors —
   so two values per line yield two observations from one 2-line group:
   (1,2) then (10,20). When the group's first line runs out of tokens the
   pointer advances to the next group. */
data paired;
  input a #2 b @@;
  datalines;
1 10
2 20
;
run;

proc print data=paired; run;

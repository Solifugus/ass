/* Implicit query pushdown over an external (database) source must be value-
   identical to local filtering. A SQLite library is bound and seeded with a row
   whose numeric value is MISSING — the case where SAS and SQL filter semantics
   diverge. Two reads exercise both sides of the pushdown decision:

     - `where=(x >= 3)` is value-safe to push (SAS orders missing below every
       number, so `>=` excludes missing exactly as SQL's NULL handling does); it
       is sent to the database, and the missing row is excluded.
     - `where=(x < 5)` is NOT pushed (SAS keeps missing for `<`, SQL would drop
       it); the table is read in full and filtered locally, so the missing row is
       kept — proving pushdown never changes results.

   `keep=id x` becomes a column projection on the safe read. @TMP@ is a fresh temp
   dir removed after the run. SQLite is the locally testable engine; the same
   pushdown path serves the Postgres/SQL Server/Oracle LIBNAME engines. */
libname db sqlite "@TMP@/pushdown.db";

data db.t;
  input id x;
  datalines;
1 10
2 .
3 3
;
run;

/* Safe operator (>=): pushed to the database; missing excluded, as in SAS. */
data safe;
  set db.t(keep=id x where=(x >= 3));
run;

/* Unsafe operator (<): filtered locally; missing kept, as in SAS. */
data unsafe;
  set db.t(where=(x < 5));
run;

proc print data=safe; run;
proc print data=unsafe; run;

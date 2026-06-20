/* Database write-back through a LIBNAME engine, end to end. A SQLite library is
   bound with `libname db sqlite "..."`; a DATA step then writes a dataset INTO
   that library (`data db.orders; set ...;`), which replaces/creates the table in
   the database file rather than the in-memory WORK store. A later step reads it
   back (`set db.orders;`), proving the round trip: numeric, character, and a
   date-formatted column (stored as a SQL DATE and recovered as a SAS date)
   survive the write/read. @TMP@ is substituted by the harness with a fresh
   temporary directory (removed after the run) so the test leaves no artifacts.
   SQLite is the locally testable engine; the same path serves the Postgres/SQL
   Server/Oracle LIBNAME engines. */
libname db sqlite "@TMP@/writeback.db";

data source;
  input id name $ amt opened : date9.;
  format opened date9.;
  datalines;
1 Acme 100 15JAN2020
2 Globex 250 03FEB2021
3 Initech 50 20MAR2019
;
run;

/* Write into the database, keeping only the rows we want there. */
data db.orders;
  set source;
  where amt >= 100;
run;

/* Read it back out of the database into WORK for verification. */
data result;
  set db.orders;
run;

proc print data=result;
run;

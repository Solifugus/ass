/* PROC SQL pass-through to an external database, end to end. A SQLite library is
   bound with `libname db sqlite "..."` and seeded with a DATA step; PROC SQL then
   sends the database its OWN native SQL through `from connection to db (...)` —
   an aggregate the in-process engine never sees — and brings the result back as a
   WORK dataset. This proves pass-through: the libref's connection is reused (no
   explicit `connect to` needed), the native query runs on the database, and its
   result set materializes in WORK for verification. @TMP@ is substituted by the
   harness with a fresh temporary directory (removed after the run) so the test
   leaves no artifacts. SQLite is the locally testable engine; the same path
   serves the Postgres/SQL Server/Oracle LIBNAME engines. */
libname db sqlite "@TMP@/passthru.db";

data db.sales;
  input region $ amt;
  datalines;
East 100
East 50
West 70
;
run;

/* Run native (database-dialect) SQL on the bound database and return the result.
   The aggregation happens in the database, not the in-process engine. */
proc sql;
  create table summary as
    select * from connection to db
      (select region, sum(amt) as total from "sales" group by region order by region);
quit;

proc print data=summary;
run;

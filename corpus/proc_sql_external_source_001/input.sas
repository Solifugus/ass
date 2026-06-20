/* PROC SQL must read an external LIBNAME member as a query SOURCE (not only via
   pass-through), and must resolve a WORK-qualified source. A SQLite library is
   bound and seeded with an `emp` table; a WORK `dept` table is built locally.

   The join draws from db.emp (external libref) and work.dept (WORK-qualified) at
   once and selects through table aliases (e.name, d.dept, e.id, d.id). Those
   alias.column references must pass through untouched while the libref-qualified
   table references are resolved: db.emp is loaded on demand from the database into
   the in-process engine, and work.dept reduces to its member name.

   A second statement reads the external table alone (work.wq) to cover the plain
   external-source case with a value-safe WHERE.

   @TMP@ is a fresh temp dir removed after the run. SQLite is the locally testable
   engine; the same source-resolution path serves the Postgres/SQL Server/Oracle/
   DB2 LIBNAME engines. Requires a CGo build. */
libname db sqlite "@TMP@/source.db";

data db.emp;
  input id name $;
  datalines;
1 Bob
2 Amy
3 Cy
;
run;

data dept;
  input id dept $;
  datalines;
1 Sales
2 Eng
3 Ops
;
run;

proc sql;
  create table work.joined as
    select e.name, d.dept
      from db.emp e join work.dept d on e.id = d.id
      order by e.name;
  create table work.wq as
    select id, name from db.emp where id >= 2 order by id;
quit;

proc print data=joined; run;
proc print data=wq; run;

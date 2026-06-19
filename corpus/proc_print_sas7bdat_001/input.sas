/* Reads native SAS dataset files (.sas7bdat) through a base/directory LIBNAME
   engine: `libname lib "@DIR@";` binds the item directory, and its members
   (airline.sas7bdat, productsales.sas7bdat) read as datasets via SET. This
   verifies the clean-room .sas7bdat reader end to end — column metadata, numeric
   and character values — against hand-derived expected values.
   @DIR@ is substituted by the harness with the item directory so paths port. */
libname lib "@DIR@";

/* productsales: filter to a single unique observation and keep a mix of numeric
   and character columns. month=12054 is the SAS date for 1993-01-01. */
data one;
  set lib.productsales;
  where country = "CANADA" and region = "EAST" and division = "EDUCATION"
        and product = "SOFA" and month = 12054;
  keep actual predict country region product year;
run;

/* airline: YEAR is stored as a (truncated) numeric and reads as clean integers
   1948..1979. */
data years;
  set lib.airline;
  keep year;
run;

proc print data=one;
run;

proc print data=years;
run;

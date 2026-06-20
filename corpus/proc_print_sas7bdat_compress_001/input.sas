/* Reads row-compressed native SAS dataset files (.sas7bdat) through the
   base/directory LIBNAME engine. rle_data.sas7bdat is RLE-compressed (SASYZCRL)
   and rdc_data.sas7bdat is RDC-compressed (SASYZCR2); both hold the same wide
   table (100 columns x 10 rows). Keeping the first three columns value-verifies
   that each row decompresses to the same observations as the uncompressed form,
   including missing numeric values and an empty (missing) character value.
   The clean-room decompressors are ports of the public reverse-engineering
   literature on the format (ReadStat's RLE command table, the sas7bdat vignette,
   and Ed Ross's published RDC algorithm) -- never from proprietary SAS material.
   @DIR@ is substituted by the harness with the item directory so paths port. */
libname lib "@DIR@";

data rle;
  set lib.rle_data;
  keep column1 column2 column3;
run;

data rdc;
  set lib.rdc_data;
  keep column1 column2 column3;
run;

proc print data=rle;
run;

proc print data=rdc;
run;

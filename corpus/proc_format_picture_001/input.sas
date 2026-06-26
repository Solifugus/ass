/* PROC FORMAT PICTURE: output-only picture templates. Digit selectors (0-9) are
   positions the value's digits fill; a 0 selector zero-suppresses leading
   positions, a nonzero selector forces printing. Message characters (comma, dot,
   dash, %) print once significant digits begin. Options: prefix=, mult=, fill=.
   put() materializes the formatted strings; strip() removes the picture's
   leading blanks so the meaningful content is value-verified. */
proc format;
  picture dollars low-high = '000,000,009.99' (prefix='$');
  picture phone   other    = '000-0000';
  picture pct     low-high = '009.9%';
  picture zid     low-high = '000000' (mult=1 fill='0');
run;

data formatted;
  input amt phone pc id;
  damt = strip(put(amt, dollars.));
  dph  = strip(put(phone, phone.));
  dpc  = strip(put(pc, pct.));
  zid  = strip(put(id, zid.));
  keep damt dph dpc zid;
  datalines;
1234.5 5551234 7.3 42
58.07 1234567 100 7
;
run;

proc print data=formatted; run;

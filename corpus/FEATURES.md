# Corpus Feature Tags

This is the **canonical list of feature tags** used in corpus item `meta.yaml` files
(`features:` list). Every tag a corpus item claims must appear here. Keep this list in
sync with the compatibility levels in [`../docs/design.md`](../docs/design.md) (§3) and the classification in §9.

Tags are lowercase, hyphenated. The "Level" column maps each tag to the compatibility
level it first becomes relevant at, matching the build order in [`../docs/PLAN.md`](../docs/PLAN.md).

| Tag | Level | Meaning |
|-----|-------|---------|
| `data-step` | 1 | A basic DATA step: `data <name>; ... run;` with the implicit row loop. |
| `input` | 1 | The `input` statement reading variables (list input; `$` marks character vars). |
| `datalines` | 1 | Inline raw data via `datalines;`/`cards;` up to a terminating `;`. |
| `infile` | 1 | External flat-file input via `infile "<path>"` (`dlm=`/`dsd`/`firstobs=`/`obs=`) feeding `input`. |
| `file-put` | 1 | External flat-file output via `file "<path>"` (`dlm=`/`dsd`) and `put` (variables, literals, inline formats); `data _null_`. |
| `libname` | 2 | `libname <ref> <engine> "<conn>";` binding a libref to an external library (base/directory of `.sas7bdat` files, or a database engine — Postgres/SQL Server/Oracle/SQLite, read and DATA-step write-back). |
| `sas7bdat` | 2 | Reading native SAS dataset files (`.sas7bdat`) as datasets (clean-room reader; 32/64-bit little-endian; uncompressed and RLE/RDC row-compressed). |
| `assignment` | 1 | Variable assignment expressions, e.g. `x = a + b;`. |
| `if-then-else` | 1 | Conditional execution `if cond then ...; else ...;` and subsetting `if cond;`. |
| `do-loop` | 1 | Iterative `do`/`do while`/`do until` ... `end;`. |
| `automatic-vars` | 1 | Automatic variables `_N_` (iteration counter) and `_ERROR_`. |
| `missing-values` | 1 | Numeric `.` and character `''` missing values and their propagation. |
| `set` | 1 | Reading rows from an existing dataset with `set <ds>;`. |
| `keep-drop` | 1 | Column selection via `keep`/`drop` statements or dataset options. |
| `dataset-options` | 5 | Dataset options `(keep= drop= rename=(o=n) where=(...))` on SET/MERGE/DATA/PROC `data=`. |
| `expressions` | 1 | Arithmetic, comparison, and logical operator evaluation. |
| `functions` | 1 | Built-in DATA step functions (`substr`, `trim`, `round`, `sum`, ...). |
| `where` | 2 | `where` filtering (DATA step option and statement), vs. subsetting `if`. |
| `proc-print` | 2 | `proc print` listing output, including `var`, `noobs`, `label`. |
| `proc-sort` | 2 | `proc sort` with `by`, `descending`, `out=`, `nodupkey`. |
| `proc-append` | 2 | `proc append base= data= [force]`: append observations to a base data set (created if absent; WORK or a database libref). |
| `proc-contents` | 2 | `proc contents` dataset metadata listing. |
| `proc-import` | 2 | `proc import` reading external files (CSV first). |
| `proc-export` | 2 | `proc export` writing external files (CSV first). |
| `proc-sql` | 3 | `proc sql` ... `quit;` block. |
| `sql-select` | 3 | `select` projection, `where`, calculated columns, table aliases. |
| `sql-join` | 3 | Inner/left/right joins in `proc sql`. |
| `sql-groupby` | 3 | `group by` with aggregate functions and `order by`. |
| `sql-create-table` | 3 | `create table <name> as select ...` materialization. |
| `sql-passthrough` | 3 | Explicit pass-through to an external database (`connect to` / `execute ... by` / `select ... from connection to`). |
| `sql-external-source` | 3 | Ordinary `proc sql` reading a libref-qualified (external or WORK) table as a query source, incl. joining an external table with a WORK table. |
| `query-pushdown` | 3 | Value-safe implicit pushdown of `keep=` (projection) and `where=` (numeric `=`/`>`/`>=`) to a database source. |
| `line-hold` | 5 | Line-pointer control: trailing line-hold on INPUT (`@@` across iterations, `@` within the iteration) and `#n` multi-line pointers reading/writing one observation across several physical lines (INPUT and PUT). |
| `macro-let` | 4 | `%let` macro-variable assignment. |
| `macro-var` | 4 | `&var` (and `&&`, `.`-terminated) macro-variable resolution. |
| `macro-def` | 4 | `%macro`/`%mend` definitions with positional/named parameters. |
| `macro-control` | 4 | Macro control flow: `%if/%then/%else`, `%do`. |
| `retain` | 5 | `retain` — values persist across iterations. |
| `sum-statement` | 5 | Sum statement `var + expr;` (retained accumulator). |
| `arrays` | 5 | `array` declarations and subscripted references. |
| `by-group` | 5 | BY-group processing with `first.`/`last.` variables. |
| `merge` | 5 | Match-merge `merge ... by ...;` with `in=` dataset flags. |
| `formats` | 5 | Output formats (`w.d`, `dollar`, `date`/`datetime`, ...). |
| `labels` | 5 | Descriptive variable labels via the `label <var>="text";` statement (DATA step and PROC); inherited through SET/MERGE; rendered by `proc print label`. |
| `informats` | 5 | Input informats for reading formatted values. |
| `dates` | 5 | Date/datetime literals (`'01JAN2020'd`) and date handling. |
| `user-formats` | 5 | User-defined formats via `proc format`. |
| `proc-format` | 5 | `proc format` VALUE statement (ranges, `low`/`high`, `other`, char). |
| `proc-means` | 6 | `proc means` descriptive statistics. |
| `proc-freq` | 6 | `proc freq` frequency tables. |
| `proc-summary` | 6 | `proc summary` aggregation. |
| `proc-reg` | 6 | `proc reg` linear regression. |
| `proc-glm` | 6 | `proc glm` general linear models. |
| `class` | 6 | CLASS categorical predictors in PROC REG/GLM (reference-cell coding). |
| `ods` | — | ODS / output destination handling (not yet scheduled). |
| `graphics` | — | Graphics procedures (out of scope for now). |
| `unsupported` | — | Vendor-specific or intentionally unsupported constructs (negative tests). |

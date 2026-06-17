# Corpus Feature Tags

This is the **canonical list of feature tags** used in corpus item `meta.yaml` files
(`features:` list). Every tag a corpus item claims must appear here. Keep this list in
sync with the compatibility levels in `ass-design.md` (§3) and the classification in §9.

Tags are lowercase, hyphenated. The "Level" column maps each tag to the compatibility
level it first becomes relevant at, matching the build order in `PLAN.md`.

| Tag | Level | Meaning |
|-----|-------|---------|
| `data-step` | 1 | A basic DATA step: `data <name>; ... run;` with the implicit row loop. |
| `input` | 1 | The `input` statement reading variables (list input; `$` marks character vars). |
| `datalines` | 1 | Inline raw data via `datalines;`/`cards;` up to a terminating `;`. |
| `assignment` | 1 | Variable assignment expressions, e.g. `x = a + b;`. |
| `if-then-else` | 1 | Conditional execution `if cond then ...; else ...;` and subsetting `if cond;`. |
| `do-loop` | 1 | Iterative `do`/`do while`/`do until` ... `end;`. |
| `automatic-vars` | 1 | Automatic variables `_N_` (iteration counter) and `_ERROR_`. |
| `missing-values` | 1 | Numeric `.` and character `''` missing values and their propagation. |
| `set` | 1 | Reading rows from an existing dataset with `set <ds>;`. |
| `keep-drop` | 1 | Column selection via `keep`/`drop` statements or dataset options. |
| `expressions` | 1 | Arithmetic, comparison, and logical operator evaluation. |
| `functions` | 1 | Built-in DATA step functions (`substr`, `trim`, `round`, `sum`, ...). |
| `where` | 2 | `where` filtering (DATA step option and statement), vs. subsetting `if`. |
| `proc-print` | 2 | `proc print` listing output, including `var`, `noobs`, `label`. |
| `proc-sort` | 2 | `proc sort` with `by`, `descending`, `out=`, `nodupkey`. |
| `proc-contents` | 2 | `proc contents` dataset metadata listing. |
| `proc-import` | 2 | `proc import` reading external files (CSV first). |
| `proc-export` | 2 | `proc export` writing external files (CSV first). |
| `proc-sql` | 3 | `proc sql` ... `quit;` block. |
| `sql-select` | 3 | `select` projection, `where`, calculated columns, table aliases. |
| `sql-join` | 3 | Inner/left/right joins in `proc sql`. |
| `sql-groupby` | 3 | `group by` with aggregate functions and `order by`. |
| `sql-create-table` | 3 | `create table <name> as select ...` materialization. |
| `macro-let` | 4 | `%let` macro-variable assignment. |
| `macro-var` | 4 | `&var` (and `&&`, `.`-terminated) macro-variable resolution. |
| `macro-def` | 4 | `%macro`/`%mend` definitions with positional/named parameters. |
| `macro-control` | 4 | Macro control flow: `%if/%then/%else`, `%do`. |
| `retain` | 5 | `retain` — values persist across iterations. |
| `arrays` | 5 | `array` declarations and subscripted references. |
| `by-group` | 5 | BY-group processing with `first.`/`last.` variables. |
| `merge` | 5 | Match-merge `merge ... by ...;` with `in=` dataset flags. |
| `formats` | 5 | Output formats (`w.d`, `dollar`, `date`/`datetime`, ...). |
| `informats` | 5 | Input informats for reading formatted values. |
| `dates` | 5 | Date/datetime literals (`'01JAN2020'd`) and date handling. |
| `user-formats` | 5 | User-defined formats via `proc format`. |
| `proc-means` | 6 | `proc means` descriptive statistics. |
| `proc-freq` | 6 | `proc freq` frequency tables. |
| `proc-summary` | 6 | `proc summary` aggregation. |
| `proc-reg` | 6 | `proc reg` linear regression. |
| `proc-glm` | 6 | `proc glm` general linear models. |
| `ods` | — | ODS / output destination handling (not yet scheduled). |
| `graphics` | — | Graphics procedures (out of scope for now). |
| `unsupported` | — | Vendor-specific or intentionally unsupported constructs (negative tests). |

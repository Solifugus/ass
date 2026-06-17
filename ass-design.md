# Analyst’s Statistical Suite

## Initial Design and Compatibility Plan

## 1. Purpose

Analyst’s Statistical Suite, abbreviated ASS, is an open-source SAS-compatible data processing and analytics system.

The first goal is not to replace every SAS product. The first goal is to run a useful subset of common SAS programs, especially DATA step, PROC SORT, PROC PRINT, PROC SQL, import/export, formats, and macro basics.

The project should prioritize real-world ETL and reporting compatibility before advanced statistical procedures.

## 2. Design Goals

ASS should be:

* Open source
* Cross-platform
* Written primarily in Go
* Usable from the command line
* Compatible with common `.sas` source files where practical
* Test-driven against a public SAS code corpus
* Designed around clear compatibility levels

## 3. Compatibility Levels

### Level 0: Parser Recognition

The system can tokenize and parse SAS-like files, identify DATA steps, PROC blocks, comments, statements, and macro directives.

### Level 1: Basic DATA Step

Supports:

* `data`
* `set`
* `input`
* `datalines`
* assignments
* `if/then`
* `do/end`
* implicit row loop
* `_N_`
* `_ERROR_`
* missing values
* character and numeric variables

### Level 2: Core Procedures

Supports:

* `proc print`
* `proc sort`
* basic `proc contents`
* basic `proc import`
* basic `proc export`

### Level 3: SQL and ETL

Supports:

* `proc sql`
* `create table as select`
* joins
* where clauses
* group by
* order by
* calculated columns
* table aliases

This may be backed internally by DuckDB, SQLite, PostgreSQL, or a Go-native execution engine.

### Level 4: Macro Basics

Supports:

* `%let`
* macro variables
* `&var` expansion
* `%macro` / `%mend`
* positional and named macro parameters
* basic `%if/%then/%else`
* basic `%do`

### Level 5: Advanced DATA Step Compatibility

Supports:

* `retain`
* arrays
* BY-group processing
* `first.` and `last.` variables
* `merge`
* `in=`
* formats and informats
* date literals
* user-defined formats

### Level 6: Statistical Procedures

Later support for selected procedures:

* `proc means`
* `proc freq`
* `proc summary`
* `proc reg`
* `proc glm`

These should be added after the DATA step and ETL core are strong.

## 4. Architecture

```text
SAS source
  ↓
Lexer
  ↓
Macro preprocessor
  ↓
Parser
  ↓
AST
  ↓
Intermediate representation
  ↓
Runtime engine / VM
  ↓
Tables, reports, logs, output files
```

## 5. Major Components

```text
ass/
  cmd/ass/              Command-line interface
  lexer/                Tokenizer
  parser/               SAS parser
  ast/                  Syntax tree definitions
  macro/                Macro preprocessor
  runtime/              DATA step runtime
  vm/                   Optional bytecode VM
  table/                Dataset abstraction
  proc/                 PROC implementations
  formats/              Formats and informats
  sql/                  PROC SQL bridge or engine
  log/                  SAS-style logging
  corpus/               Test corpus metadata
  tests/                Unit and compatibility tests
```

## 6. Execution Model

ASS should behave like SAS at the step level.

A source file is processed as a sequence of steps:

```sas
data example;
  ...
run;

proc print data=example;
run;
```

Each DATA or PROC block is parsed, compiled to an internal representation, then executed.

The DATA step runtime should model the SAS Program Data Vector concept. Each row iteration updates variables, applies statements, and outputs rows according to SAS-like rules.

## 7. Dataset Model

Datasets should support:

* library name
* dataset name
* columns
* column types
* labels
* formats
* informats
* rows
* missing values
* metadata

Initial storage may be in-memory. Later storage may include:

* local ASS dataset files
* CSV
* Parquet
* DuckDB
* SQLite
* PostgreSQL

## 8. Test Corpus Plan

The project should build a public compatibility corpus from several legal, public sources.

Initial sources:

1. SAS official code examples repository
2. SAS Support DATA step samples
3. SAS Communities code examples
4. Public SAS macro repositories
5. Small hand-written regression tests
6. User-contributed real-world examples, with proprietary data removed

Each corpus item should have metadata:

```yaml
id: data_step_basic_001
source: sas-code-examples
license: upstream license
features:
  - data-step
  - datalines
  - proc-print
expected:
  parse: pass
  execute: pass
  output: normalized-table
priority: 1
```

## 9. Corpus Classification

Examples should be classified by feature:

```text
DATA step basics
DATA step input/datalines
Assignments and expressions
IF/THEN/ELSE
DO loops
SET
MERGE
BY groups
Formats
Informats
Dates
PROC PRINT
PROC SORT
PROC SQL
PROC IMPORT
PROC EXPORT
Macros
Statistical PROCs
ODS/output
Graphics
Unsupported/vendor-specific
```

## 10. Compatibility Harness

The test harness should support several modes:

```bash
ass test corpus/
ass test --parse-only corpus/
ass test --feature data-step
ass test --feature proc-sort
ass test --compare-output expected/
```

Each test should report:

```text
parsed: yes/no
executed: yes/no
log compatible: yes/no
output compatible: yes/no
unsupported features: list
```

The project should publish compatibility percentages:

```text
DATA step basics:     84%
PROC PRINT:          100%
PROC SORT:            92%
PROC SQL:             41%
Macro language:        8%
Overall corpus:       37%
```

## 11. Legal and Licensing Notes

ASS should avoid copying proprietary SAS documentation, source code, examples without compatible licenses, branding, or internal implementation details.

The project should aim for behavioral compatibility based on public examples, user tests, and clean-room implementation.

The name should clearly state that ASS is not affiliated with SAS Institute.

Suggested wording:

> Analyst’s Statistical Suite is an independent open-source project. It is not affiliated with, endorsed by, or sponsored by SAS Institute Inc.

## 12. Initial Milestones

### Milestone 1: CLI and Lexer

* `ass file.sas`
* tokenize source
* recognize comments, strings, identifiers, numbers, semicolons
* detect DATA and PROC steps

### Milestone 2: Parser

* parse DATA step blocks
* parse PROC blocks
* produce AST
* parse simple expressions

### Milestone 3: Minimal DATA Step Runtime

Support:

```sas
data people;
  input name $ age;
  datalines;
John 25
Mary 30
;
run;
```

### Milestone 4: PROC PRINT

Support:

```sas
proc print data=people;
run;
```

### Milestone 5: Expressions and Filtering

Support:

```sas
data adults;
  set people;
  if age >= 18;
run;
```

### Milestone 6: PROC SORT

Support:

```sas
proc sort data=people;
  by age;
run;
```

### Milestone 7: Basic PROC SQL

Support:

```sas
proc sql;
  create table adults as
  select *
  from people
  where age >= 18;
quit;
```

### Milestone 8: Corpus Harness

* import public examples
* tag by feature
* run parse tests
* run execution tests
* produce compatibility report

## 13. Guiding Principle

The project should not begin as a complete SAS clone.

It should begin as a practical SAS-compatible ETL engine, then grow one tested feature at a time.

The heart of the system is the DATA step runtime. Everything else can orbit that sun.

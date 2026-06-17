# ASS — Analyst's Statistical Suite

ASS is an open-source, SAS-compatible data processing and analytics engine written in Go and driven from the command line. It aims for **behavioral compatibility** with a practical subset of SAS programs — the DATA step, PROC SORT/PRINT/SQL, import/export, formats, and macro basics — prioritizing real-world ETL and reporting over advanced statistics.

See [`ass-design.md`](ass-design.md) for the full design and [`PLAN.md`](PLAN.md) for the development roadmap.

## Status

Early development. The project is being built one tested, corpus-backed feature at a time, following the compatibility levels in the design document.

## Usage

```bash
ass <file.sas>       # run a SAS program
ass test <dir>       # run the compatibility corpus (planned)
```

## Disclaimer

Analyst's Statistical Suite is an independent open-source project. It is not affiliated with, endorsed by, or sponsored by SAS Institute Inc. "SAS" is a trademark of SAS Institute Inc. ASS implements behavioral compatibility through clean-room methods based on public examples.

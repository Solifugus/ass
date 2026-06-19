# ASS documentation

Project documentation for **Analyst's Statistical Suite (ASS)**. Start with the
[project README](../README.md) for an overview, usage, and the feature list.

| Document | What it covers |
|----------|----------------|
| [`design.md`](design.md) | Design rationale: what ASS is, the interpreter pipeline, execution model, and compatibility levels (L0–L6). |
| [`databases.md`](databases.md) | Reading external databases via the `LIBNAME` engine (Postgres/SQL Server/Oracle): assignment, type mapping, and limitations. |
| [`COMPATIBILITY.md`](COMPATIBILITY.md) | What "compatible" means here (value/result compatibility, not byte-identical output), the per-feature compatibility matrix, and known deferrals. |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | How to add a corpus item, run the harness, and implement a new PROC or function (clean-room rules included). |
| [`PLAN.md`](PLAN.md) | The living, resumable development plan and dated progress log — the source of truth for what's done. |
| [`RELEASE.md`](RELEASE.md) | Release checklist, the pre-release green-corpus gate, and per-platform CGo build notes. |

Related references that live next to the code they describe:

- [`../CLAUDE.md`](../CLAUDE.md) — architecture notes and guidance for working in the repo.
- [`../corpus/README.md`](../corpus/README.md) — corpus item format and the value-verification model.
- [`../corpus/FEATURES.md`](../corpus/FEATURES.md) — canonical feature-tag catalog.
- [`../sql/DECISION.md`](../sql/DECISION.md) — why PROC SQL embeds SQLite via CGo.

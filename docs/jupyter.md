# Jupyter kernel

ASS ships a Jupyter kernel so you can run SAS-compatible programs cell by cell in
Jupyter Notebook / JupyterLab, with **session state persisting across cells** —
datasets, librefs, and macro variables/definitions created in one cell are
visible in the next, exactly as in `ass repl`. The kernel is the second consumer
of the resident [session model](design.md) (§16); a cell is one
`session.Submit`.

## Install

Build `ass`, then register the kernelspec for your user:

```bash
go build -o ass ./cmd/ass     # CGO_ENABLED=0 works — see "No C compiler" below
./ass kernel --install
```

This writes a kernelspec (`kernel.json`) into your Jupyter data directory
(`$JUPYTER_DATA_DIR`, else `$XDG_DATA_HOME/jupyter`, else
`~/.local/share/jupyter` on Linux / `~/Library/Jupyter` on macOS). The `argv`
points at the exact `ass` binary you ran `--install` with, so keep that binary in
place (or re-run `--install` after moving it).

Then start Jupyter and pick the **"ASS (SAS)"** kernel:

```bash
jupyter lab          # or: jupyter notebook
```

## Using it

Type SAS into a cell and run it. The cell output is the merged SAS LOG + listing
in execution order:

- The **log** renders as a SAS-style colored monospace block — `NOTE` in blue,
  `WARNING` in green, `ERROR` in red — so the familiar SAS log reads at a glance.
- Tabular results — **PROC PRINT / MEANS / FREQ (one-way) / SQL / REG** — render
  as **styled HTML tables**: a caption with the name and `rows × cols`, a shaded
  header, zebra-striped rows, and right-aligned tabular-figure numbers.
- **PROC FREQ cross-tabs** render as a contingency table — each cell stacks the
  frequency and the cell/row/column percentages, with row, column, and grand
  totals in the margins.
- **PROC PROOF** renders as a pass/fail panel — a header with the passed/failed
  tally and a status dot, then one row per assertion with a colored PASS / FAIL /
  N-RUN pill, the violations-over-checked count, and the offending observations.
- **PROC REG** shows a model-summary line (dependent variable, R², N) above the
  parameter-estimates table, and the **chi-square** statistic (PROC FREQ
  `/ chisq`) renders as its own small table.
- **TITLE** statements (`title`, `title2`…`title10`, and `title;` to clear) are
  shown as headings above procedure output, and **FOOTNOTE** statements
  (`footnote`…`footnote10`) as a dimmed block below it; both persist across cells.

The styling uses grayscale overlay tints and inherits the theme's text color, so
tables and the log look right on **both light and dark** notebook themes without
any configuration. State carries across cells:

```sas
/* cell 1 */
%let cutoff = 100;
data sales;
  input region $ amt;
  datalines;
east 120
west 80
;
run;
```

```sas
/* cell 2 — sees SALES and &cutoff from cell 1 */
proc print data=sales; where amt > &cutoff; run;
```

A cell that logs an `ERROR` (e.g. a failing `PROC PROOF` assertion) or fails to
parse is reported as an error cell; the log/diagnostics appear in the output.

## How it works

```
Jupyter frontend  ──ZeroMQ (shell/iopub/stdin/control/hb)──►  ass kernel
                                                                  │
                                          execute_request → session.Submit(code)
                                                                  │
                                        captured LOG+listing → iopub "stream" (stdout)
                                                  status/result → execute_reply
```

- **Wire protocol:** Jupyter messaging v5.3. Messages are HMAC-SHA256 signed with
  the key from the connection file; the kernel verifies inbound signatures and
  signs every reply. Implemented in `kernel/message.go` (framing + signing) and
  `kernel/connection.go` (connection file).
- **Sockets** (`kernel/kernel.go`): shell/control/stdin are ROUTER, iopub is PUB,
  heartbeat is REP — all bound by the kernel. Each request is wrapped in the
  protocol's `busy`/`idle` status pair on iopub.
- **Output capture & rich rendering:** the kernel attaches a *rich sink*
  (`log.NewSink`) to the logger, so all output is delivered as ordered `Event`s
  (`log`, `listing`, `table`) instead of to the stdout streams. The kernel
  batches `log`/`listing` text into a colored monospace block (`renderLogHTML`)
  and emits each tabular PROC result (an `Event` of kind `table`, carrying both a
  plain-text and a styled-HTML rendering) as `display_data` with `text/html` + a
  `text/plain` fallback. Events arrive in execution order, so a NOTE, then a
  table, then the next NOTE interleave correctly. **Outside a rich frontend the
  sink is never attached** — `log.New(w)` keeps the listing on stdout as before,
  and `Logger.EmitTable` falls through to writing plain text — so batch (`ass
  file.sas`) and REPL output is byte-identical to before this feature. The table
  HTML is produced by `proc.renderHTMLListing`, which reuses the exact column
  selection / label / format logic of the text `renderListing` (and HTML-escapes
  all cell values).

## Previewing the look without Jupyter

The kernel test can dump a representative cell rendering (colored log + styled
tables) to an HTML file you can open in any browser:

```bash
ASS_WRITE_SAMPLE=/tmp/ass-sample.html go test ./kernel/ -run SampleHTML
```

## No C compiler

The kernel uses the **pure-Go** ZeroMQ implementation
[`github.com/go-zeromq/zmq4`](https://github.com/go-zeromq/zmq4), so the whole
binary — kernel included — builds with `CGO_ENABLED=0` into a static executable
(`goczmq` is an indirect dependency only compiled under the `czmq4` build tag,
which ASS never sets). This matches the project's CGo-free default
([`sql/DECISION.md`](../sql/DECISION.md), [`design.md`](design.md) §15).

## Limitations (v1)

- PROC PRINT/MEANS/FREQ (one-way, cross-tab, chi-square)/SQL/REG and PROC PROOF
  render as HTML, with TITLE headings. Anything else a PROC writes falls back to
  plain text in the colored log block.
- No `stdin`/`input_request` round-trip — SAS programs are non-interactive, so
  the stdin socket is bound but unused.
- Titles/footnotes render left-aligned (SAS centers them).
- `interrupt_request` is acknowledged but does not yet abort a running step
  (steps are typically short); cooperative cancellation is future work.
- Tab-completion (`complete_request`) and introspection (`inspect_request`) are
  not yet implemented.

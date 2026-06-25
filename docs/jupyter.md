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
(NOTEs, then PROC tables, in execution order). State carries across cells:

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
- **Output capture:** the runtime's two streams — the LOG (`log.Logger`) and the
  procedure listing (`Logger.Listing()`, formerly hard-wired to stdout) — are
  pointed at one buffer per cell via `log.NewWith`, so PROC output is captured
  instead of escaping to the kernel process's stdout, and the two streams stay in
  execution order.

## No C compiler

The kernel uses the **pure-Go** ZeroMQ implementation
[`github.com/go-zeromq/zmq4`](https://github.com/go-zeromq/zmq4), so the whole
binary — kernel included — builds with `CGO_ENABLED=0` into a static executable
(`goczmq` is an indirect dependency only compiled under the `czmq4` build tag,
which ASS never sets). This matches the project's CGo-free default
([`sql/DECISION.md`](../sql/DECISION.md), [`design.md`](design.md) §15).

## Limitations (v1)

- Output is streamed as plain text (`text/plain`). Rich `display_data` (HTML
  tables for PROC PRINT, etc.) is a forward-looking enhancement.
- No `stdin`/`input_request` round-trip — SAS programs are non-interactive, so
  the stdin socket is bound but unused.
- `interrupt_request` is acknowledged but does not yet abort a running step
  (steps are typically short); cooperative cancellation is future work.
- Tab-completion (`complete_request`) and introspection (`inspect_request`) are
  not yet implemented.

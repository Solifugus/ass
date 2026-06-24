# Performance — DATA-step baseline and the bytecode-VM decision

This note records the first DATA-step performance baseline, the cheap interpreter
wins applied on top of it, and — the point of the exercise — the **evidence-based
decision on whether the bytecode VM is worth building** (see [`design.md`](design.md)
§14 for the architecture commitment to the interpreter family).

## Benchmarks

`runtime/bench_test.go`, 50 000 rows each, on an Intel Core Ultra 7 155H:

- **`BenchmarkDataStepSetTransform`** — the representative ETL path: an implicit
  loop over a `set` source applying arithmetic, an `if/then`, and `keep=`.
  Exercises `ResetVars`, the expression evaluator, PDV get/set, and `writeRow`.
- **`BenchmarkDataStepGenerate`** — compute-bound: a single iteration with an
  explicit `do` loop emitting rows of arithmetic + `if/then` + `output`. Isolates
  eval + `writeRow` from the SET path.

Run them with:

```bash
go test ./runtime/ -run='^$' -bench=BenchmarkDataStep -benchmem
```

## Baseline → cheap win

| Benchmark | Baseline | After `writeRow` plan caching | Speedup | allocs/op |
|-----------|----------|-------------------------------|---------|-----------|
| SetTransform (50k rows) | ~757 ms (~66k rows/s) | ~555 ms (~90k rows/s) | ~1.37× | 150k → 100k |
| Generate (50k rows)     | ~473 ms (~106k rows/s) | ~315 ms (~159k rows/s) | ~1.50× | 150k → 100k |

**The win:** `writeRow` was rebuilding the output column set on *every row* —
`table.Dataset.AddColumn` does a linear, `strings.ToLower`-ing scan of existing
columns, so output was O(rows × vars²) with per-row `strings.ToLower` and a fresh
`pdv.Names()` slice allocation each row. It now computes the output-column plan
**once** (rebuilt only when the PDV variable set grows — typically never after the
first row) and the per-row path just copies values by a precomputed lowercased
key (`PDV.GetLower`). This removed the 33%-of-runtime `writeRow` hotspot and one
allocation per row.

## Where the time still goes (profile after the win)

CPU profile of SetTransform is now dominated by **GC** (`gcDrain` + `scanobject`
≈ 35–40% cumulative), driven by allocations, with the rest spread across
expression eval (`Eval`/`evalInfix` ≈ 10%) and map operations (PDV hashing +
`strings.ToLower` ≈ 10%). The remaining ~2 allocations per row are **the output
`table.Row` map** (an `hmap` + its bucket array) — `Row` is a `map[string]Value`.

So the next tier of cost is *structural*, not incidental:

1. **Per-row map allocation.** Every output row is a `map[string]Value`. The
   allocation + GC of one map per row is the single largest remaining cost.
2. **PDV as a map keyed by lowercased name.** Every variable read/write hashes a
   string (and historically lowercased it). A compiled step knows its variables
   up front and could address them by slot index into a slice.

## Decision: defer the bytecode VM; the cost is data representation, not dispatch

The benchmark confirms the per-row loop — not parsing, I/O, or PROC dispatch — is
the bottleneck, which is the precondition the roadmap set for considering the VM.
But the profile also shows **bytecode dispatch is not where the time goes.** The
costs are (a) allocating a map per row and (b) hashing strings for PDV access. A
bytecode VM that still represented rows as maps and the PDV as a string map would
not move these numbers; conversely, the two changes that *would* give the next
5–10× are independent of bytecode:

- **PDV as registers** — resolve variable names to slot indices at compile time,
  store values in a `[]Value`, and have `Eval`/assignments address slots directly.
- **Rows without per-row maps** — a slice-backed row (or a columnar output
  buffer) instead of `map[string]Value`.

Therefore:

- **Do not build the bytecode VM yet.** Its headline benefit (cheap opcode
  dispatch) does not address the measured bottleneck.
- **When DATA-step throughput becomes a real constraint** (a workload needing
  well above ~100k rows/s), the first structural move is the **slot-indexed PDV +
  non-map row representation**. That refactor stands on its own and would also be
  the foundation a future bytecode VM or vectorized executor builds on — so it is
  the correct next performance investment, ahead of any VM.
- The current tree-walking interpreter remains the **correctness oracle** (the
  corpus differentially validates any future faster path against it).

Until then, ~90–160k rows/s is adequate for the ETL/reporting workloads ASS
targets, and effort is better spent on compatibility breadth and the data-quality
/ interactivity differentiators.

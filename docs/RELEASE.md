# Release checklist

ASS is versioned with semantic-version git tags (`vMAJOR.MINOR.PATCH`). Until a
`1.0.0`, the API/CLI may change between minor versions.

## Pre-release gate

1. `go build ./...` and `go vet ./...` are clean.
2. `go test ./...` passes.
3. `ass test corpus/` is green (exit 0, 100%). This is the hard gate.
4. [`PLAN.md`](PLAN.md), [`../README.md`](../README.md), and [`COMPATIBILITY.md`](COMPATIBILITY.md)
   reflect what ships (regenerate the matrix: `ass test corpus/`).

## Tagging

```bash
git tag -a vX.Y.Z -m "ASS vX.Y.Z"
git push origin vX.Y.Z
```

## Build artifacts

ASS embeds SQLite via CGo, so binaries are **per-platform** and need a C
toolchain for the target. Build natively on each OS/arch (or use a CGo-capable
cross toolchain / container):

```bash
CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o ass ./cmd/ass
```

Recommended targets: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64,
windows/amd64. A pure-Go SQLite driver (e.g. `modernc.org/sqlite`) would enable
`CGO_ENABLED=0` cross-builds; see [`../sql/DECISION.md`](../sql/DECISION.md) for the trade-off.

## License

MIT (`LICENSE`). Confirm the copyright year/holder before tagging.

package runtime

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/table"
)

// buildBig builds a WORK.big dataset of n rows with a few typed columns, used as
// the SET source for the per-row-loop benchmarks. The cost of building it is not
// part of any benchmark's timed region.
func buildBig(n int) *table.Library {
	lib := table.NewLibrary()
	ds := table.NewDataset("WORK", "big")
	ds.AddColumn(table.Column{Name: "id", Kind: table.Numeric})
	ds.AddColumn(table.Column{Name: "amt", Kind: table.Numeric})
	ds.AddColumn(table.Column{Name: "grp", Kind: table.Character})
	grps := []string{"east", "west", "north", "south"}
	for i := 0; i < n; i++ {
		ds.AppendRow(table.Row{
			"id":  table.Num(float64(i)),
			"amt": table.Num(float64(i%1000) + 0.5),
			"grp": table.Char(grps[i%len(grps)]),
		})
	}
	lib.Put(ds)
	return lib
}

const benchRows = 50000

// BenchmarkDataStepSetTransform is the representative ETL path: an implicit loop
// over a SET source applying arithmetic, an if/then, and keep=. It exercises
// ResetVars, the expression evaluator, PDV get/set, and writeRow per row.
func BenchmarkDataStepSetTransform(b *testing.B) {
	src := `data out;
  set big;
  tax = amt * 0.07;
  total = amt + tax;
  if amt > 500 then tier = 2; else tier = 1;
  keep id total tier grp;
run;`
	prog := parser.New(src).ParseProgram()
	lib := buildBig(benchRows)
	logger := log.New(&bytes.Buffer{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := RunProgram(prog, lib, logger); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	rowsPerSec := float64(benchRows) * float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(rowsPerSec/1e6, "Mrows/s")
}

// BenchmarkDataStepGenerate is a compute-bound generator: a single iteration with
// an explicit do-loop emitting benchRows rows of arithmetic + if/then + output.
// It isolates eval + writeRow from the SET path.
func BenchmarkDataStepGenerate(b *testing.B) {
	src := fmt.Sprintf(`data out;
  do i = 1 to %d;
    x = i * 2;
    y = x + 3;
    if x > 100 then flag = 1; else flag = 0;
    output;
  end;
run;`, benchRows)
	prog := parser.New(src).ParseProgram()
	logger := log.New(&bytes.Buffer{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lib := table.NewLibrary()
		if err := RunProgram(prog, lib, logger); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	rowsPerSec := float64(benchRows) * float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(rowsPerSec/1e6, "Mrows/s")
}

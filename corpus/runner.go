package corpus

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/macro"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/runtime"
	"github.com/solifugus/ass/table"
)

// Options controls a test run.
type Options struct {
	ParseOnly bool   // only check parsing, never execute
	Feature   string // if set, only run items tagged with this feature
}

// Result is the outcome of running one item.
type Result struct {
	Item       Item
	ParsePass  bool // parse outcome matched meta.expected.parse
	Executed   bool // execution was attempted
	ExecPass   bool // execute outcome matched meta.expected.execute
	Skipped    bool // execution intentionally skipped (expected.execute: skip)
	OutChecked bool // expected_output.txt existed and was compared
	OutPass    bool // captured output matched expected_output.txt
	Detail     string
}

// Pass reports whether the item met all the expectations that were checked.
func (r Result) Pass() bool {
	if !r.ParsePass {
		return false
	}
	if r.Executed && !r.ExecPass {
		return false
	}
	if r.OutChecked && !r.OutPass {
		return false
	}
	return true
}

// Report is the aggregate of a test run.
type Report struct {
	Results []Result
}

// Run executes the test for each item (subject to opts) and returns a report.
func Run(items []Item, opts Options) Report {
	var rep Report
	for _, it := range items {
		if opts.Feature != "" && !it.HasFeature(opts.Feature) {
			continue
		}
		rep.Results = append(rep.Results, runItem(it, opts))
	}
	return rep
}

func runItem(it Item, opts Options) Result {
	res := Result{Item: it}

	expanded := macro.Process(it.Input)
	p := parser.New(expanded)
	prog := p.ParseProgram()
	parseOK := len(p.Errors()) == 0
	expectParse := it.Expected.Parse != "fail" // default: expect pass
	res.ParsePass = parseOK == expectParse
	if !res.ParsePass {
		res.Detail = fmt.Sprintf("parse: got ok=%v, expected %s; %s",
			parseOK, orDefault(it.Expected.Parse, "pass"), strings.Join(p.Errors(), "; "))
	}

	if opts.ParseOnly || it.Expected.Execute == "skip" {
		res.Skipped = it.Expected.Execute == "skip"
		return res
	}
	if !parseOK {
		// Can't execute what didn't parse; ExecPass stays false unless a parse
		// failure was expected (then execution isn't meaningful).
		res.Executed = true
		return res
	}

	res.Executed = true
	var logBuf bytes.Buffer
	logger := log.New(&logBuf)
	lib := table.NewLibrary()
	out, err := captureStdout(func() error { return runtime.RunProgram(prog, lib, logger) })
	execOK := err == nil
	expectExec := it.Expected.Execute != "fail"
	res.ExecPass = execOK == expectExec
	if !res.ExecPass {
		res.Detail = fmt.Sprintf("execute: got ok=%v (%v), expected %s",
			execOK, err, orDefault(it.Expected.Execute, "pass"))
	}

	if it.Expected.Output == "verified" && it.ExpectedOutput != "" {
		res.OutChecked = true
		res.OutPass = out == it.ExpectedOutput
		if !res.OutPass {
			res.Detail = "output mismatch"
		}
	}
	return res
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what was
// written (so a PROC's listing can be captured for comparison).
func captureStdout(fn func() error) (string, error) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", fn()
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var b bytes.Buffer
		io.Copy(&b, r)
		done <- b.String()
	}()
	runErr := fn()
	w.Close()
	os.Stdout = old
	return <-done, runErr
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// --- reporting ---

// Summary tallies overall pass counts.
func (rep Report) Summary() (total, parsed, executed, passed int) {
	total = len(rep.Results)
	for _, r := range rep.Results {
		if r.ParsePass {
			parsed++
		}
		if r.Executed && r.ExecPass {
			executed++
		}
		if r.Pass() {
			passed++
		}
	}
	return
}

// FeatureStats returns per-feature pass/total counts, sorted by feature name.
type FeatureStat struct {
	Feature string
	Pass    int
	Total   int
}

func (rep Report) FeatureStats() []FeatureStat {
	totals := map[string]int{}
	passes := map[string]int{}
	for _, r := range rep.Results {
		for _, f := range r.Item.Features {
			totals[f]++
			if r.Pass() {
				passes[f]++
			}
		}
	}
	var out []FeatureStat
	for f, t := range totals {
		out = append(out, FeatureStat{Feature: f, Pass: passes[f], Total: t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Feature < out[j].Feature })
	return out
}

// WriteReport prints a per-item report followed by the per-feature and overall
// compatibility summary tables.
func (rep Report) WriteReport(w io.Writer, verbose bool) {
	for _, r := range rep.Results {
		status := "PASS"
		if !r.Pass() {
			status = "FAIL"
		}
		fmt.Fprintf(w, "%-4s %s\n", status, r.Item.ID)
		if verbose && r.Detail != "" {
			fmt.Fprintf(w, "       %s\n", r.Detail)
		}
	}

	fmt.Fprintln(w, "\nPer-feature compatibility:")
	for _, fs := range rep.FeatureStats() {
		fmt.Fprintf(w, "  %-22s %3d/%-3d  %5.1f%%\n",
			fs.Feature, fs.Pass, fs.Total, pct(fs.Pass, fs.Total))
	}

	total, parsed, executed, passed := rep.Summary()
	fmt.Fprintf(w, "\nTotals: %d items | parsed %d (%.1f%%) | executed %d (%.1f%%) | passed %d (%.1f%%)\n",
		total, parsed, pct(parsed, total), executed, pct(executed, total), passed, pct(passed, total))
}

func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}

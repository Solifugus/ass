package corpus

import (
	"bytes"
	"encoding/json"
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
	ValChecked bool // expected.datasets declared and value-compared
	ValPass    bool // produced dataset values matched the expected values
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
	if r.ValChecked && !r.ValPass {
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

	// @DIR@ resolves to the item's directory so a program can reference a
	// companion data file (e.g. `infile "@DIR@/data.csv";`) portably. @TMP@
	// resolves to a fresh per-item temp directory (removed after the run) for
	// programs that write files (e.g. `file "@TMP@/out.csv";`) and read them back.
	input := it.Input
	if it.Dir != "" {
		input = strings.ReplaceAll(input, "@DIR@", it.Dir)
	}
	if strings.Contains(input, "@TMP@") {
		tmp, err := os.MkdirTemp("", "ass-corpus-")
		if err == nil {
			defer os.RemoveAll(tmp)
			input = strings.ReplaceAll(input, "@TMP@", tmp)
		}
	}
	expanded := macro.Process(input)
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

	// Value compatibility: compare produced datasets against the hand-derived
	// expected values. This is the primary correctness bar (see corpus/README.md).
	if execOK && len(it.Expected.Datasets) > 0 {
		res.ValChecked = true
		res.ValPass = true
		for _, name := range sortedKeys(it.Expected.Datasets) {
			if ok, detail := checkDataset(lib, name, it.Expected.Datasets[name]); !ok {
				res.ValPass = false
				res.Detail = detail
				break
			}
		}
	}
	return res
}

// sortedKeys returns the dataset names in deterministic order.
func sortedKeys(m map[string]ExpectedDataset) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// checkDataset compares one produced dataset in lib against its expected
// columns and row values. It returns a failure detail on the first mismatch.
func checkDataset(lib *table.Library, name string, exp ExpectedDataset) (bool, string) {
	ds, ok := lib.Get(name)
	if !ok {
		return false, fmt.Sprintf("dataset %s not found in library", strings.ToUpper(name))
	}
	cols := exp.Columns
	if len(cols) > 0 {
		got := ds.ColumnNames()
		if !sameColumns(got, cols) {
			return false, fmt.Sprintf("%s columns = %v, want %v", name, got, cols)
		}
	} else {
		cols = ds.ColumnNames()
	}
	if len(ds.Rows) != len(exp.Rows) {
		return false, fmt.Sprintf("%s nobs = %d, want %d", name, len(ds.Rows), len(exp.Rows))
	}
	for i, erow := range exp.Rows {
		for j, ecell := range erow {
			if j >= len(cols) {
				break
			}
			act := ds.Get(ds.Rows[i], cols[j])
			if !valueMatches(ecell, act) {
				return false, fmt.Sprintf("%s row %d %s: got %q, want %v",
					name, i+1, cols[j], act.Display(), ecell)
			}
		}
	}
	return true, ""
}

// sameColumns reports whether two column-name lists match in order
// (case-insensitive).
func sameColumns(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if !strings.EqualFold(got[i], want[i]) {
			return false
		}
	}
	return true
}

// valueMatches compares one expected cell (from YAML: number, string, "." or
// null) against a produced table.Value. Numbers compare with tolerance; strings
// compare exactly against character values (or the display of a numeric); "."
// and null mean missing.
func valueMatches(exp interface{}, act table.Value) bool {
	switch e := exp.(type) {
	case nil:
		return act.IsMissing()
	case bool:
		n := 0.0
		if e {
			n = 1
		}
		return !act.IsMissing() && act.Kind == table.Numeric && act.Num == n
	case int:
		return numMatch(float64(e), act)
	case int64:
		return numMatch(float64(e), act)
	case float64:
		return numMatch(e, act)
	case string:
		if e == "." {
			return act.IsMissing()
		}
		if act.IsMissing() {
			return false
		}
		if act.Kind == table.Character {
			return act.Str == e
		}
		return act.Display() == e
	}
	return false
}

func numMatch(want float64, act table.Value) bool {
	if act.IsMissing() || act.Kind != table.Numeric {
		return false
	}
	d := act.Num - want
	if d < 0 {
		d = -d
	}
	tol := 1e-6
	if a := absf(want); a > 1 {
		tol = 1e-9 * a
	}
	return d <= tol
}

func absf(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
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

// FeatureStats returns per-feature counts, sorted by feature name. Verified is
// the number of items carrying the feature that value-verify (declare
// expected.datasets and match) — the metric that drives the corpus-backfill
// backlog (a feature with Verified == 0 has no value-checked coverage).
type FeatureStat struct {
	Feature  string `json:"feature"`
	Pass     int    `json:"pass"`
	Total    int    `json:"total"`
	Verified int    `json:"verified"`
}

func (rep Report) FeatureStats() []FeatureStat {
	totals := map[string]int{}
	passes := map[string]int{}
	verified := map[string]int{}
	for _, r := range rep.Results {
		for _, f := range r.Item.Features {
			totals[f]++
			if r.Pass() {
				passes[f]++
			}
			if r.ValChecked && r.ValPass {
				verified[f]++
			}
		}
	}
	var out []FeatureStat
	for f, t := range totals {
		out = append(out, FeatureStat{Feature: f, Pass: passes[f], Total: t, Verified: verified[f]})
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
	valChecked, valPass := 0, 0
	for _, r := range rep.Results {
		if r.ValChecked {
			valChecked++
			if r.ValPass {
				valPass++
			}
		}
	}
	fmt.Fprintf(w, "\nTotals: %d items | parsed %d (%.1f%%) | executed %d (%.1f%%) | passed %d (%.1f%%)\n",
		total, parsed, pct(parsed, total), executed, pct(executed, total), passed, pct(passed, total))
	fmt.Fprintf(w, "Value-verified: %d/%d items declare expected dataset values and match (%.1f%%)\n",
		valPass, valChecked, pct(valPass, valChecked))
}

// WriteCoverage prints the corpus value-verification backlog: per feature, the
// number of items that value-verify out of the total carrying that feature, with
// features that have NO value-verified coverage listed first (the gaps to fill).
// This is the Phase-13.5 backfill worklist made visible and measurable.
func (rep Report) WriteCoverage(w io.Writer) {
	stats := rep.FeatureStats()
	// Gaps first (Verified == 0), then by ascending coverage ratio, then name.
	sort.Slice(stats, func(i, j int) bool {
		gi, gj := stats[i].Verified == 0, stats[j].Verified == 0
		if gi != gj {
			return gi // zero-verified features sort first
		}
		ri, rj := pct(stats[i].Verified, stats[i].Total), pct(stats[j].Verified, stats[j].Total)
		if ri != rj {
			return ri < rj
		}
		return stats[i].Feature < stats[j].Feature
	})

	gaps := 0
	fmt.Fprintln(w, "Corpus value-verification coverage (verified / total items per feature):")
	for _, fs := range stats {
		flag := "  "
		if fs.Verified == 0 {
			flag = "!!" // no value-verified coverage at all
			gaps++
		}
		fmt.Fprintf(w, "  %s %-22s %3d/%-3d  %5.1f%%\n",
			flag, fs.Feature, fs.Verified, fs.Total, pct(fs.Verified, fs.Total))
	}

	valChecked, valPass := 0, 0
	for _, r := range rep.Results {
		if r.ValChecked {
			valChecked++
			if r.ValPass {
				valPass++
			}
		}
	}
	fmt.Fprintf(w, "\n%d of %d features have NO value-verified item (marked !!) — the backfill backlog.\n",
		gaps, len(stats))
	fmt.Fprintf(w, "Items value-verified: %d/%d (%.1f%%).\n", valPass, len(rep.Results), pct(valPass, len(rep.Results)))
}

func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}

// jsonReport is the machine-readable shape of a corpus run.
type jsonReport struct {
	Summary  jsonSummary   `json:"summary"`
	Features []FeatureStat `json:"features"`
	Items    []jsonItem    `json:"items"`
}

type jsonSummary struct {
	Total         int `json:"total"`
	Parsed        int `json:"parsed"`
	Executed      int `json:"executed"`
	Passed        int `json:"passed"`
	ValueChecked  int `json:"value_checked"`
	ValueVerified int `json:"value_verified"`
}

type jsonItem struct {
	ID            string   `json:"id"`
	Pass          bool     `json:"pass"`
	ParsePass     bool     `json:"parse_pass"`
	Executed      bool     `json:"executed"`
	ExecPass      bool     `json:"exec_pass"`
	Skipped       bool     `json:"skipped"`
	OutputChecked bool     `json:"output_checked"`
	OutputPass    bool     `json:"output_pass"`
	ValueChecked  bool     `json:"value_checked"`
	ValuePass     bool     `json:"value_pass"`
	Features      []string `json:"features,omitempty"`
	Detail        string   `json:"detail,omitempty"`
}

// WriteJSON writes the report as machine-readable JSON (a stable schema for CI
// and tooling): an overall summary, per-feature pass/total counts, and a
// per-item record. The same data the text report shows, without presentation.
func (rep Report) WriteJSON(w io.Writer) error {
	total, parsed, executed, passed := rep.Summary()
	valChecked, valPass := 0, 0
	for _, r := range rep.Results {
		if r.ValChecked {
			valChecked++
			if r.ValPass {
				valPass++
			}
		}
	}
	out := jsonReport{
		Summary: jsonSummary{
			Total: total, Parsed: parsed, Executed: executed, Passed: passed,
			ValueChecked: valChecked, ValueVerified: valPass,
		},
		Features: rep.FeatureStats(),
	}
	for _, r := range rep.Results {
		out.Items = append(out.Items, jsonItem{
			ID:            r.Item.ID,
			Pass:          r.Pass(),
			ParsePass:     r.ParsePass,
			Executed:      r.Executed,
			ExecPass:      r.ExecPass,
			Skipped:       r.Skipped,
			OutputChecked: r.OutChecked,
			OutputPass:    r.OutPass,
			ValueChecked:  r.ValChecked,
			ValuePass:     r.ValPass,
			Features:      r.Item.Features,
			Detail:        r.Detail,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

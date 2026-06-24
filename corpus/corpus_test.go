package corpus

import (
	"bytes"
	"encoding/json"
	"testing"
)

// The tests load the real corpus items in this directory (".").

func TestLoadCorpus(t *testing.T) {
	items, err := Load(".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(items) < 10 {
		t.Fatalf("loaded %d items, expected the full corpus", len(items))
	}
	for _, it := range items {
		if it.ID == "" {
			t.Errorf("item in %s has empty ID", it.Dir)
		}
		if it.Input == "" {
			t.Errorf("item %s has empty input.sas", it.ID)
		}
		if len(it.Features) == 0 {
			t.Errorf("item %s has no features", it.ID)
		}
	}
}

func TestRunCorpusAllPass(t *testing.T) {
	items, err := Load(".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rep := Run(items, Options{})
	for _, r := range rep.Results {
		if !r.Pass() {
			t.Errorf("item %s failed: %s", r.Item.ID, r.Detail)
		}
	}
	total, parsed, executed, passed := rep.Summary()
	skipped := 0
	for _, r := range rep.Results {
		if r.Skipped {
			skipped++
		}
	}
	// Skipped items (those with expected.execute: skip) are parsed and counted
	// as passing, but not executed.
	if parsed != total || executed != total-skipped || passed != total {
		t.Errorf("expected all %d to pass; parsed=%d executed=%d passed=%d skipped=%d",
			total, parsed, executed, passed, skipped)
	}
}

func TestRunFeatureFilter(t *testing.T) {
	items, _ := Load(".")
	rep := Run(items, Options{Feature: "proc-sql"})
	if len(rep.Results) == 0 {
		t.Fatal("feature filter returned no items")
	}
	for _, r := range rep.Results {
		if !r.Item.HasFeature("proc-sql") {
			t.Errorf("item %s lacks the filtered feature", r.Item.ID)
		}
	}
}

func TestParseOnlySkipsExecution(t *testing.T) {
	items, _ := Load(".")
	rep := Run(items, Options{ParseOnly: true})
	for _, r := range rep.Results {
		if r.Executed {
			t.Errorf("item %s executed under --parse-only", r.Item.ID)
		}
	}
}

// TestValueVerificationCatchesMismatch proves the value-comparison harness fails
// when produced dataset values differ from the declared expectations (not just
// that correct items pass).
func TestValueVerificationCatchesMismatch(t *testing.T) {
	src := "data t;\n  input id v;\n  datalines;\n1 10\n2 20\n;\nrun;"

	good := Item{
		ID:    "vv_good",
		Input: src,
		Expected: Expected{Parse: "pass", Execute: "pass", Datasets: map[string]ExpectedDataset{
			"t": {Columns: []string{"id", "v"}, Rows: [][]interface{}{{1, 10}, {2, 20}}},
		}},
	}
	r := runItem(good, Options{})
	if !r.ValChecked || !r.ValPass || !r.Pass() {
		t.Errorf("correct values should pass: %+v (%s)", r, r.Detail)
	}

	badVal := good
	badVal.ID = "vv_badval"
	badVal.Expected.Datasets = map[string]ExpectedDataset{
		"t": {Columns: []string{"id", "v"}, Rows: [][]interface{}{{1, 10}, {2, 999}}},
	}
	r = runItem(badVal, Options{})
	if r.ValPass || r.Pass() {
		t.Errorf("wrong value should fail; detail=%q", r.Detail)
	}

	badShape := good
	badShape.ID = "vv_badshape"
	badShape.Expected.Datasets = map[string]ExpectedDataset{
		"t": {Rows: [][]interface{}{{1, 10}}}, // wrong row count
	}
	r = runItem(badShape, Options{})
	if r.ValPass || r.Pass() {
		t.Errorf("wrong nobs should fail; detail=%q", r.Detail)
	}

	missing := good
	missing.ID = "vv_missing"
	missing.Expected.Datasets = map[string]ExpectedDataset{
		"nope": {Rows: [][]interface{}{{1}}},
	}
	r = runItem(missing, Options{})
	if r.ValPass {
		t.Errorf("missing dataset should fail; detail=%q", r.Detail)
	}
}

func TestWriteJSON(t *testing.T) {
	items, err := Load(".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rep := Run(items, Options{})
	var buf bytes.Buffer
	if err := rep.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var got jsonReport
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("JSON did not round-trip: %v", err)
	}
	total, parsed, executed, passed := rep.Summary()
	if got.Summary.Total != total || got.Summary.Parsed != parsed ||
		got.Summary.Executed != executed || got.Summary.Passed != passed {
		t.Errorf("summary mismatch: json=%+v want total=%d parsed=%d executed=%d passed=%d",
			got.Summary, total, parsed, executed, passed)
	}
	if len(got.Items) != len(rep.Results) {
		t.Errorf("items = %d, want %d", len(got.Items), len(rep.Results))
	}
	if len(got.Features) != len(rep.FeatureStats()) {
		t.Errorf("features = %d, want %d", len(got.Features), len(rep.FeatureStats()))
	}
}

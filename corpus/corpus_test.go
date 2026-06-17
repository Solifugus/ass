package corpus

import "testing"

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
	if parsed != total || executed != total || passed != total {
		t.Errorf("expected all %d to pass; parsed=%d executed=%d passed=%d",
			total, parsed, executed, passed)
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

package session

import (
	"strings"
	"testing"

	"github.com/solifugus/ass/log"
)

// submit runs one fragment and fails the test on any error.
func submit(t *testing.T, s *Session, src string) string {
	t.Helper()
	var b strings.Builder
	if err := s.Submit(src, log.New(&b)); err != nil {
		t.Fatalf("Submit(%q) error: %v", src, err)
	}
	return b.String()
}

func TestSessionDatasetPersistsAcrossSubmissions(t *testing.T) {
	s := New()
	// First fragment builds a dataset.
	submit(t, s, "data a;\n  input x;\n  datalines;\n1\n2\n3\n;\nrun;")
	// A later fragment reads it back — proving WORK persists across submissions.
	submit(t, s, "data b;\n  set a;\n  y = x * 10;\nrun;")

	ds, ok := s.Lib.Get("b")
	if !ok {
		t.Fatalf("dataset B not found; session did not carry A across submissions")
	}
	if ds.NObs() != 3 {
		t.Fatalf("B has %d obs, want 3", ds.NObs())
	}
	got := ds.Get(ds.Rows[2], "y")
	if got.Num != 30 {
		t.Errorf("B.y[3] = %v, want 30", got.Num)
	}
}

func TestSessionMacroVarsPersist(t *testing.T) {
	s := New()
	// A %let in one fragment...
	submit(t, s, "%let n = 5;")
	// ...is visible to a &reference in a later one.
	submit(t, s, "data c;\n  v = &n * 2;\noutput;\nrun;")

	ds, ok := s.Lib.Get("c")
	if !ok || ds.NObs() != 1 {
		t.Fatalf("dataset C not built; macro var did not persist (ok=%v)", ok)
	}
	if got := ds.Get(ds.Rows[0], "v"); got.Num != 10 {
		t.Errorf("C.v = %v, want 10 (&n should be 5)", got.Num)
	}
}

func TestSessionMacroDefinitionPersists(t *testing.T) {
	s := New()
	submit(t, s, "%macro tens(v); &v * 10 %mend;")
	submit(t, s, "data d;\n  w = %tens(4);\noutput;\nrun;")

	ds, ok := s.Lib.Get("d")
	if !ok || ds.NObs() != 1 {
		t.Fatalf("dataset D not built; macro def did not persist (ok=%v)", ok)
	}
	if got := ds.Get(ds.Rows[0], "w"); got.Num != 40 {
		t.Errorf("D.w = %v, want 40", got.Num)
	}
}

func TestSessionParseErrorLeavesStateIntact(t *testing.T) {
	s := New()
	submit(t, s, "data keep_me;\n  x = 1;\noutput;\nrun;")

	// A garbled fragment must fail to parse and execute nothing.
	err := s.Submit("data oops;\n  x = ;;;; @@@\n", log.New(&strings.Builder{}))
	if err == nil {
		t.Fatalf("expected a parse error for garbled fragment")
	}
	if _, ok := err.(*ParseError); !ok {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}

	// The earlier dataset survives.
	if !s.Lib.Has("keep_me") {
		t.Errorf("KEEP_ME lost after a failed submission")
	}
}

func TestSessionLibnamePersists(t *testing.T) {
	dir := t.TempDir()
	s := New()
	// Bind a base libref in one fragment...
	submit(t, s, "libname store \""+dir+"\";")
	// ...and a later, unrelated submission must still see the binding, proving
	// librefs are session state, not per-fragment.
	submit(t, s, "data scratch;\n  k = 7;\noutput;\nrun;")

	if _, ok := s.Lib.Backend("store"); !ok {
		t.Errorf("libref STORE did not persist across submissions")
	}
}

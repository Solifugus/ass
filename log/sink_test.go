package log

import (
	"strings"
	"testing"
)

// TestSinkRoutesAllOutputAsEvents verifies that a sink receives log lines,
// listing writes, and tables as ordered events, and that the text writers are
// not used when a sink is attached.
func TestSinkRoutesAllOutputAsEvents(t *testing.T) {
	var events []Event
	l := NewSink(func(e Event) { events = append(events, e) })

	if !l.Rich() {
		t.Fatal("Rich() should be true for a sink logger")
	}

	l.Note("hello %d", 1)
	l.Listing().Write([]byte("plain text"))
	l.EmitTable("TXT", "<table></table>")

	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(events), events)
	}
	if events[0].Kind != "log" || !strings.Contains(events[0].Text, "NOTE: hello 1") {
		t.Errorf("event[0] = %+v, want a NOTE log event", events[0])
	}
	if events[1].Kind != "listing" || events[1].Text != "plain text" {
		t.Errorf("event[1] = %+v, want a listing event", events[1])
	}
	if events[2].Kind != "table" || events[2].Text != "TXT" || events[2].HTML != "<table></table>" {
		t.Errorf("event[2] = %+v, want a table event carrying both renderings", events[2])
	}
}

// TestNoSinkWritesPlainText verifies the batch/REPL path: with no sink, EmitTable
// writes the plain text to the listing writer and nothing is routed elsewhere,
// so output is byte-identical to writing renderListing directly.
func TestNoSinkWritesPlainText(t *testing.T) {
	var lst, logw strings.Builder
	l := NewWith(&logw, &lst)

	if l.Rich() {
		t.Fatal("Rich() should be false without a sink")
	}
	l.EmitTable("the table text", "<table>ignored</table>")
	l.Note("a note")

	if lst.String() != "the table text" {
		t.Errorf("listing = %q, want %q (HTML must not leak)", lst.String(), "the table text")
	}
	if got := logw.String(); got != "NOTE: a note\n" {
		t.Errorf("log = %q, want the NOTE line", got)
	}
}

// TestErrorCountWithSink confirms error counting still works under a sink (the
// kernel relies on it to mark a cell failed).
func TestErrorCountWithSink(t *testing.T) {
	l := NewSink(func(Event) {})
	l.Error("boom")
	l.Error("boom again")
	if l.ErrorCount() != 2 {
		t.Errorf("ErrorCount = %d, want 2", l.ErrorCount())
	}
}

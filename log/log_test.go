package log

import (
	"strings"
	"testing"
)

func TestLoggerLevels(t *testing.T) {
	var b strings.Builder
	l := New(&b)
	l.Note("hello %s", "world")
	l.Warning("careful")
	l.Error("boom %d", 42)
	got := b.String()
	want := "NOTE: hello world\nWARNING: careful\nERROR: boom 42\n"
	if got != want {
		t.Errorf("log output =\n%q\nwant\n%q", got, want)
	}
}

func TestDatasetNote(t *testing.T) {
	var b strings.Builder
	New(&b).DatasetNote("work", "people", 3, 2)
	want := "NOTE: The data set WORK.PEOPLE has 3 observations and 2 variables.\n"
	if b.String() != want {
		t.Errorf("DatasetNote = %q, want %q", b.String(), want)
	}
}

func TestNilLoggerIsSafe(t *testing.T) {
	var l *Logger
	// Must not panic.
	l.Note("ignored")
	l.DatasetNote("work", "x", 0, 0)
}

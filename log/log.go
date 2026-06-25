package log

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Logger writes SAS-style log lines (NOTE/WARNING/ERROR) to an underlying
// writer. SAS prefixes informational lines with "NOTE: ", warnings with
// "WARNING: ", and errors with "ERROR: ". A nil Logger is usable and discards
// everything, so callers need not guard every call.
//
// SAS keeps two output streams: the LOG (NOTE/WARNING/ERROR/PUT, written here)
// and the procedure listing (LST, the PROC output). The Logger carries both —
// w is the LOG; lst is the listing, reachable via Listing(). The batch CLI sends
// the LOG to stderr and the listing to stdout; the Jupyter kernel captures both.
//
// When a rich sink is attached (NewSink), all output — log lines, listings, and
// tabular PROC results — is delivered to it as ordered Events instead of being
// written to w/lst, so a frontend (the Jupyter kernel) can render tables as HTML
// while keeping everything in execution order. With no sink (New/NewWith), the
// sink path is never taken and behavior is exactly the plain-text streams, so
// batch and REPL output is unchanged.
type Logger struct {
	w    io.Writer
	lst  io.Writer
	sink func(Event)
	errs int
}

// Event is one ordered output item produced during a run, for a rich sink.
// Kind is "log" (a NOTE/WARNING/ERROR/PUT line), "listing" (plain PROC text with
// no richer form), or "table" (a tabular PROC result carrying both a plain-text
// rendering in Text and an HTML rendering in HTML).
type Event struct {
	Kind string
	Text string
	HTML string
}

// New creates a Logger writing the LOG to w and the procedure listing to stdout
// (the batch default).
func New(w io.Writer) *Logger { return &Logger{w: w} }

// NewWith creates a Logger with separate LOG (logw) and listing (lstw) writers,
// for callers that capture the procedure output rather than letting it reach
// stdout — e.g. the Jupyter kernel. New(w) is equivalent to NewWith(w, stdout).
func NewWith(logw, lstw io.Writer) *Logger { return &Logger{w: logw, lst: lstw} }

// NewSink creates a Logger that delivers all output to sink as ordered Events
// rather than to text writers. The Jupyter kernel uses this to interleave
// streamed log/listing text with HTML tables.
func NewSink(sink func(Event)) *Logger { return &Logger{sink: sink} }

// Rich reports whether a rich (Event) sink is attached, so callers can skip
// building an HTML rendering when only plain text will be consumed.
func (l *Logger) Rich() bool { return l != nil && l.sink != nil }

// EmitTable outputs a tabular PROC result. With a rich sink attached it delivers
// a "table" Event carrying both renderings (the frontend chooses HTML); without
// one it writes the plain text to the listing stream, so batch/REPL output is
// byte-identical to writing the listing directly. Pass html only when Rich().
func (l *Logger) EmitTable(text, html string) {
	if l == nil {
		return
	}
	if l.sink != nil {
		l.sink(Event{Kind: "table", Text: text, HTML: html})
		return
	}
	fmt.Fprint(l.Listing(), text)
}

// Listing returns the writer for procedure output (the LST stream). It defaults
// to os.Stdout when unset; a nil Logger discards output. PROC implementations
// write their listings here instead of to os.Stdout directly, so the output is
// redirectable (kernel, tests, future web UI).
func (l *Logger) Listing() io.Writer {
	if l == nil {
		return io.Discard
	}
	if l.sink != nil {
		return sinkWriter{l}
	}
	if l.lst == nil {
		return os.Stdout
	}
	return l.lst
}

// sinkWriter adapts the listing io.Writer onto the rich sink: each write becomes
// a "listing" Event, so plain-text PROC output (e.g. a PROC REG header, a FREQ
// cross-tab) stays interleaved with everything else in execution order.
type sinkWriter struct{ l *Logger }

func (w sinkWriter) Write(p []byte) (int, error) {
	w.l.sink(Event{Kind: "listing", Text: string(p)})
	return len(p), nil
}

// ErrorCount returns the number of ERROR lines emitted. The CLI uses it to set a
// non-zero exit status when a run logged errors (e.g. a failing PROC PROOF
// assertion) without aborting the program. A nil Logger reports 0.
func (l *Logger) ErrorCount() int {
	if l == nil {
		return 0
	}
	return l.errs
}

// Note writes a "NOTE: ..." line (printf-style).
func (l *Logger) Note(format string, args ...any) { l.line("NOTE: ", format, args...) }

// Warning writes a "WARNING: ..." line.
func (l *Logger) Warning(format string, args ...any) { l.line("WARNING: ", format, args...) }

// Error writes an "ERROR: ..." line.
func (l *Logger) Error(format string, args ...any) {
	if l != nil {
		l.errs++
	}
	l.line("ERROR: ", format, args...)
}

func (l *Logger) line(prefix, format string, args ...any) {
	if l == nil {
		return
	}
	if l.sink != nil {
		l.sink(Event{Kind: "log", Text: fmt.Sprintf(prefix+format+"\n", args...)})
		return
	}
	if l.w == nil {
		return
	}
	fmt.Fprintf(l.w, prefix+format+"\n", args...)
}

// Put writes a raw line (no prefix) to the log, as the DATA step PUT statement
// does when no FILE destination is active. SAS PUT output is unprefixed.
func (l *Logger) Put(line string) {
	if l == nil {
		return
	}
	if l.sink != nil {
		l.sink(Event{Kind: "log", Text: line + "\n"})
		return
	}
	if l.w == nil {
		return
	}
	fmt.Fprintln(l.w, line)
}

// DatasetNote emits the standard post-step note describing an output dataset,
// e.g. "NOTE: The data set WORK.PEOPLE has 3 observations and 2 variables.".
// lib and name are upper-cased and joined as SAS displays them.
func (l *Logger) DatasetNote(lib, name string, nobs, nvars int) {
	full := strings.ToUpper(lib) + "." + strings.ToUpper(name)
	l.Note("The data set %s has %d observations and %d variables.", full, nobs, nvars)
}

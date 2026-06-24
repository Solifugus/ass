package log

import (
	"fmt"
	"io"
	"strings"
)

// Logger writes SAS-style log lines (NOTE/WARNING/ERROR) to an underlying
// writer. SAS prefixes informational lines with "NOTE: ", warnings with
// "WARNING: ", and errors with "ERROR: ". A nil Logger is usable and discards
// everything, so callers need not guard every call.
type Logger struct {
	w    io.Writer
	errs int
}

// New creates a Logger writing to w.
func New(w io.Writer) *Logger { return &Logger{w: w} }

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
	if l == nil || l.w == nil {
		return
	}
	fmt.Fprintf(l.w, prefix+format+"\n", args...)
}

// Put writes a raw line (no prefix) to the log, as the DATA step PUT statement
// does when no FILE destination is active. SAS PUT output is unprefixed.
func (l *Logger) Put(line string) {
	if l == nil || l.w == nil {
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

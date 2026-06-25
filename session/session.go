// Package session turns the batch runner ("parse file → run → exit") into a
// long-lived interpreter that holds the library and macro symbol tables and
// accepts successive program fragments. This resident session model is the
// keystone for interactive use — a REPL and the Jupyter kernel both submit
// fragments to a Session and read back the log plus the persisted library.
//
// State that persists across submissions: datasets and librefs (the
// table.Library), and macro variables and macro definitions (the
// macro.Processor). Each Submit is otherwise an ordinary program run — macro
// expand → parse → execute, step at a time — so a fragment sees everything left
// behind by earlier fragments, exactly as if they had been one file.
package session

import (
	"strings"

	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/macro"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/runtime"
	"github.com/solifugus/ass/table"
)

// Session is a resident interpreter: persistent library + macro state across
// successive Submit calls. The zero value is not usable; call New.
type Session struct {
	Lib   *table.Library
	macro *macro.Processor
}

// New creates an empty session with a fresh WORK library and macro state.
func New() *Session {
	return &Session{
		Lib:   table.NewLibrary(),
		macro: macro.New(),
	}
}

// ParseError reports that a submission failed to parse. The fragment is not
// executed; the session state is unchanged.
type ParseError struct {
	Errors []string
}

func (e *ParseError) Error() string {
	if len(e.Errors) == 1 {
		return "parse error: " + e.Errors[0]
	}
	return "parse errors:\n  - " + strings.Join(e.Errors, "\n  - ")
}

// Submit macro-expands, parses, and executes one program fragment against the
// persistent session state, writing the SAS log to logger. On a parse error it
// returns a *ParseError and runs nothing. A runtime error from a step is
// returned as-is; like the batch runner, a step that merely logs errors (e.g. a
// failing PROC PROOF assertion) returns nil here — callers inspect
// logger.ErrorCount for the gating outcome.
func (s *Session) Submit(src string, logger *log.Logger) error {
	expanded := s.macro.Process(src)
	p := parser.New(expanded)
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		return &ParseError{Errors: errs}
	}
	return runtime.RunProgram(prog, s.Lib, logger)
}

// Datasets returns the names of datasets currently in the WORK library, for
// introspection (REPL prompts, notebook explorer panels).
func (s *Session) Datasets() []string {
	return s.Lib.Names()
}

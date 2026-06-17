package proc

import (
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

// Proc is one PROC implementation. It runs against the library (reading and/or
// writing datasets), guided by its step AST, and may emit log lines.
type Proc interface {
	Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error
}

// registry maps a lower-cased PROC name to its implementation. Implementations
// register themselves from an init() so that importing the proc package wires
// them up.
var registry = map[string]Proc{}

// Register associates a PROC name with an implementation. It panics on a
// duplicate registration, which would indicate a programming error.
func Register(name string, p Proc) {
	key := strings.ToLower(name)
	if _, dup := registry[key]; dup {
		panic("proc: duplicate registration for " + key)
	}
	registry[key] = p
}

// Lookup returns the implementation registered for a PROC name, if any.
func Lookup(name string) (Proc, bool) {
	p, ok := registry[strings.ToLower(name)]
	return p, ok
}

// Run dispatches a PROC step to its registered implementation. An unregistered
// PROC is not an error: it logs a NOTE and is skipped, so an otherwise valid
// program keeps running.
func Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	p, ok := Lookup(step.Name)
	if !ok {
		logger.Note("PROC %s is not supported and was skipped.", strings.ToUpper(step.Name))
		return nil
	}
	return p.Run(lib, step, logger)
}

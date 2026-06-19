package runtime

import (
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/dbio"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/proc"
	"github.com/solifugus/ass/table"
)

// runLibname executes a global LIBNAME statement: it binds (or clears) a libref
// to an external database engine for the rest of the program.
func runLibname(s *ast.LibnameStatement, lib *table.Library, logger *log.Logger) error {
	if s.Clear {
		lib.Unassign(s.Libref)
		logger.Note("Libref %s has been deassigned.", strings.ToUpper(s.Libref))
		return nil
	}
	backend, err := dbio.Open(s.Engine, s.Connection)
	if err != nil {
		logger.Error("LIBNAME %s (%s): %v", strings.ToUpper(s.Libref), s.Engine, err)
		return nil // a failed libref is logged, like SAS; the program continues
	}
	lib.Assign(s.Libref, backend)
	logger.Note("Libref %s was successfully assigned (engine: %s).",
		strings.ToUpper(s.Libref), s.Engine)
	return nil
}

// RunProgram executes every step of a parsed program in order against a single
// library: DATA steps run on the DATA step runtime, PROC steps dispatch through
// the proc registry. Steps share data only through the library, matching SAS's
// step-at-a-time model. Execution stops at the first error.
func RunProgram(prog *ast.Program, lib *table.Library, logger *log.Logger) error {
	for _, step := range prog.Steps {
		switch s := step.(type) {
		case *ast.DataStep:
			if err := RunDataStep(s, lib, logger); err != nil {
				return err
			}
		case *ast.ProcStep:
			if err := runProcStep(s, lib, logger); err != nil {
				return err
			}
		case *ast.LibnameStatement:
			if err := runLibname(s, lib, logger); err != nil {
				return err
			}
		}
	}
	return nil
}

// runProcStep dispatches a PROC step. If its data= source needs preprocessing —
// dataset options, or resolution from an external LIBNAME engine — the resolved
// dataset is registered under a temporary WORK name the proc reads from (and
// removed afterward), so individual PROCs need no awareness of dataset options
// or external libraries.
func runProcStep(s *ast.ProcStep, lib *table.Library, logger *log.Logger) error {
	external := lib.IsExternal(s.Data)
	if s.DataOptions.IsEmpty() && !external {
		return proc.Run(lib, s, logger)
	}
	src, ok, err := lib.Resolve(s.Data)
	if err != nil {
		return err
	}
	if !ok {
		return proc.Run(lib, s, logger) // let the proc report the missing dataset
	}
	view := src
	if !s.DataOptions.IsEmpty() {
		view, err = applyDatasetOptions(src, s.DataOptions)
		if err != nil {
			return err
		}
	}
	tmp := "_dataopt_" + datasetMember(s.Data)
	view.Name = tmp
	lib.Put(view)
	defer lib.Delete(tmp)

	clone := *s
	clone.Data = tmp
	clone.DataOptions = nil
	return proc.Run(lib, &clone, logger)
}

// datasetMember returns the member component of a possibly-qualified name.
func datasetMember(name string) string {
	if i := lastDot(name); i >= 0 {
		return name[i+1:]
	}
	return name
}

func lastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

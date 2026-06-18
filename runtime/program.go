package runtime

import (
	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/proc"
	"github.com/solifugus/ass/table"
)

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
		}
	}
	return nil
}

// runProcStep dispatches a PROC step, first applying any dataset options on its
// data= source. The filtered view is registered under a temporary name the proc
// reads from, then removed so it does not leak into the library.
func runProcStep(s *ast.ProcStep, lib *table.Library, logger *log.Logger) error {
	if s.DataOptions.IsEmpty() {
		return proc.Run(lib, s, logger)
	}
	src, ok := lib.Get(s.Data)
	if !ok {
		return proc.Run(lib, s, logger) // let the proc report the missing dataset
	}
	view, err := applyDatasetOptions(src, s.DataOptions)
	if err != nil {
		return err
	}
	tmp := "_dataopt_" + s.Data
	view.Name = tmp
	lib.Put(view)
	defer lib.Delete(tmp)

	clone := *s
	clone.Data = tmp
	clone.DataOptions = nil
	return proc.Run(lib, &clone, logger)
}

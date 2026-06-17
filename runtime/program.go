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
			if err := proc.Run(lib, s, logger); err != nil {
				return err
			}
		}
	}
	return nil
}

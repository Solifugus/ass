package runtime

import (
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/table"
)

// Automatic PDV variables that exist during execution but are not written to the
// output dataset.
var automaticVars = map[string]bool{"_n_": true, "_error_": true}

// flow signals how statement execution should proceed within an iteration.
type flow int

const (
	flowNormal flow = iota // continue to the next statement
	flowDelete             // abandon this row: skip remaining statements and any output
)

// dataStep holds the per-execution state of one DATA step.
type dataStep struct {
	lib      *table.Library
	pdv      *PDV
	outputs  []*table.Dataset // datasets this step writes
	explicit bool             // step contains at least one OUTPUT statement
	n        int              // current iteration (_N_)
}

// RunDataStep executes a DATA step against the library, creating its output
// dataset(s) in the library. This phase supports steps with no input source: the
// implicit loop runs exactly one iteration. Assignment and OUTPUT statements are
// executed; other statement kinds are handled in later phases.
func RunDataStep(ds *ast.DataStep, lib *table.Library) error {
	d := &dataStep{lib: lib, pdv: NewPDV()}

	// Resolve output datasets. An unnamed DATA step writes to WORK.DATA1 in SAS;
	// we mirror that default.
	names := ds.Datasets
	if len(names) == 0 {
		names = []string{"DATA1"}
	}
	for _, name := range names {
		d.outputs = append(d.outputs, table.NewDataset("", datasetName(name)))
	}

	d.explicit = containsOutput(ds.Body)

	// Implicit row loop. With no input source, the step runs a single iteration.
	d.n = 1
	d.pdv.Set("_n_", table.Num(float64(d.n)))
	d.pdv.Set("_error_", table.Num(0))

	f, err := d.execStatements(ds.Body)
	if err != nil {
		return err
	}
	// Implicit output at the bottom of the iteration, unless the row was deleted
	// or the step manages output explicitly.
	if f != flowDelete && !d.explicit {
		d.outputAll()
	}

	for _, out := range d.outputs {
		d.lib.Put(out)
	}
	return nil
}

// execStatements runs a slice of statements in order, stopping early if one
// signals flowDelete.
func (d *dataStep) execStatements(stmts []ast.Statement) (flow, error) {
	for _, s := range stmts {
		f, err := d.execStatement(s)
		if err != nil {
			return flowNormal, err
		}
		if f == flowDelete {
			return flowDelete, nil
		}
	}
	return flowNormal, nil
}

// execStatement executes a single statement. Statement kinds not yet supported
// are ignored (they are added in later phases).
func (d *dataStep) execStatement(s ast.Statement) (flow, error) {
	switch st := s.(type) {
	case *ast.AssignmentStatement:
		v, err := Eval(st.Value, d.pdv)
		if err != nil {
			return flowNormal, err
		}
		d.pdv.Set(st.Name, v)
		return flowNormal, nil
	case *ast.OutputStatement:
		d.output(st.Datasets)
		return flowNormal, nil
	default:
		// Unsupported in this phase; skip.
		return flowNormal, nil
	}
}

// output writes the current PDV to the named output datasets, or to all of the
// step's outputs when names is empty (bare `output;`).
func (d *dataStep) output(names []string) {
	if len(names) == 0 {
		d.outputAll()
		return
	}
	for _, name := range names {
		key := strings.ToUpper(datasetName(name))
		for _, out := range d.outputs {
			if strings.ToUpper(out.Name) == key {
				d.writeRow(out)
			}
		}
	}
}

// outputAll writes the current PDV to every output dataset.
func (d *dataStep) outputAll() {
	for _, out := range d.outputs {
		d.writeRow(out)
	}
}

// writeRow appends the current PDV (minus automatic variables) as a row to ds,
// declaring any new columns in PDV order.
func (d *dataStep) writeRow(ds *table.Dataset) {
	row := make(table.Row)
	for _, name := range d.pdv.Names() {
		if automaticVars[strings.ToLower(name)] {
			continue
		}
		v := d.pdv.Get(name)
		ds.AddColumn(table.Column{Name: name, Kind: v.Kind})
		row[strings.ToLower(name)] = v
	}
	ds.AppendRow(row)
}

// containsOutput reports whether any statement in the (possibly nested) body is
// an OUTPUT statement, determining whether implicit output applies.
func containsOutput(stmts []ast.Statement) bool {
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.OutputStatement:
			return true
		case *ast.IfStatement:
			if st.Consequence != nil && containsOutput([]ast.Statement{st.Consequence}) {
				return true
			}
			if st.Alternative != nil && containsOutput([]ast.Statement{st.Alternative}) {
				return true
			}
		case *ast.DoStatement:
			if containsOutput(st.Body) {
				return true
			}
		}
	}
	return false
}

// datasetName strips a library qualifier from a dataset reference
// ("work.people" -> "people").
func datasetName(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

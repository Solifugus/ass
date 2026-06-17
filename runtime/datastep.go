package runtime

import (
	"strconv"
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
	records  []string         // inline data lines (datalines)
	recPtr   int              // next datalines record to read
	setRows  []sourceRow      // rows from SET input datasets (concatenated)
	setPtr   int              // next SET row to read
}

// sourceRow is one input row from a SET dataset, paired with the dataset so the
// reader knows each column's declared type.
type sourceRow struct {
	row table.Row
	ds  *table.Dataset
}

// RunDataStep executes a DATA step against the library, creating its output
// dataset(s) in the library. It supports input-less steps (one implicit-loop
// iteration) and inline data via INPUT + DATALINES (one iteration per data
// record). Assignment, INPUT, DATALINES, and OUTPUT statements are executed;
// other statement kinds are handled in later phases.
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
	d.records = collectDatalines(ds.Body)
	hasInput := hasInputStatement(ds.Body)

	if hasSetStatement(ds.Body) {
		// Dataset-driven loop: one iteration per input row across all SET datasets.
		d.setRows = d.collectSetRows(ds.Body)
		for d.setPtr < len(d.setRows) {
			start := d.setPtr
			if err := d.runIteration(ds.Body); err != nil {
				return err
			}
			if d.setPtr == start { // safety: ensure progress if no SET executed
				d.setPtr++
			}
		}
	} else if hasInput && len(d.records) > 0 {
		// Read-driven loop: one iteration per data record. INPUT advances recPtr;
		// the loop ends when the records are exhausted.
		for d.recPtr < len(d.records) {
			start := d.recPtr
			if err := d.runIteration(ds.Body); err != nil {
				return err
			}
			if d.recPtr == start { // safety: ensure progress if no INPUT executed
				d.recPtr++
			}
		}
	} else {
		// No input source: a single iteration.
		if err := d.runIteration(ds.Body); err != nil {
			return err
		}
	}

	for _, out := range d.outputs {
		d.lib.Put(out)
	}
	return nil
}

// runIteration runs one pass of the implicit loop: reset the PDV, set automatic
// variables, execute the body, and perform implicit output unless suppressed.
func (d *dataStep) runIteration(body []ast.Statement) error {
	d.pdv.ResetVars()
	d.n++
	d.pdv.Set("_n_", table.Num(float64(d.n)))
	d.pdv.Set("_error_", table.Num(0))

	f, err := d.execStatements(body)
	if err != nil {
		return err
	}
	if f != flowDelete && !d.explicit {
		d.outputAll()
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
	case *ast.InputStatement:
		if d.recPtr >= len(d.records) {
			// No more records: stop this iteration without output.
			return flowDelete, nil
		}
		d.applyInput(st, d.records[d.recPtr])
		d.recPtr++
		return flowNormal, nil
	case *ast.DatalinesStatement:
		// The data is collected up front; the statement itself does nothing.
		return flowNormal, nil
	case *ast.SetStatement:
		if d.setPtr >= len(d.setRows) {
			return flowDelete, nil
		}
		d.applySet(d.setRows[d.setPtr])
		d.setPtr++
		return flowNormal, nil
	case *ast.IfStatement:
		cond, err := Eval(st.Condition, d.pdv)
		if err != nil {
			return flowNormal, err
		}
		if truthy(cond) {
			return d.execStatement(st.Consequence)
		}
		if st.Alternative != nil {
			return d.execStatement(st.Alternative)
		}
		return flowNormal, nil
	case *ast.SubsettingIf:
		cond, err := Eval(st.Condition, d.pdv)
		if err != nil {
			return flowNormal, err
		}
		if !truthy(cond) {
			return flowDelete, nil // drop this row
		}
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

// applyInput parses a data record using list input (whitespace-delimited fields,
// matched positionally to the INPUT variable list) and stores the values in the
// PDV. A `$` variable is character; otherwise the field is parsed as a number.
// Missing fields, "." , and unparseable numbers become the typed missing value.
func (d *dataStep) applyInput(st *ast.InputStatement, line string) {
	fields := strings.Fields(line)
	for i, v := range st.Vars {
		var val table.Value
		switch {
		case i >= len(fields):
			if v.Char {
				val = table.MissingChar()
			} else {
				val = table.MissingNum()
			}
		case v.Char:
			val = table.Char(fields[i])
		default:
			val = parseNum(fields[i])
		}
		d.pdv.Set(v.Name, val)
	}
}

// parseNum converts an input field to a numeric value, yielding numeric missing
// for "." , empty, or unparseable text.
func parseNum(s string) table.Value {
	if s == "" || s == "." {
		return table.MissingNum()
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return table.MissingNum()
	}
	return table.Num(f)
}

// applySet seeds the PDV from a SET input row, declaring each variable with its
// source column type and order so the output preserves the input layout.
func (d *dataStep) applySet(sr sourceRow) {
	for _, c := range sr.ds.Columns {
		d.pdv.Set(c.Name, sr.ds.Get(sr.row, c.Name))
	}
}

// collectSetRows resolves the SET statement's datasets from the library and
// returns their rows concatenated in statement order (SAS reads each dataset to
// completion before the next). Unknown datasets are skipped.
func (d *dataStep) collectSetRows(stmts []ast.Statement) []sourceRow {
	var rows []sourceRow
	for _, s := range stmts {
		set, ok := s.(*ast.SetStatement)
		if !ok {
			continue
		}
		for _, name := range set.Datasets {
			src, found := d.lib.Get(name)
			if !found {
				continue
			}
			for _, r := range src.Rows {
				rows = append(rows, sourceRow{row: r, ds: src})
			}
		}
	}
	return rows
}

// hasSetStatement reports whether the body contains a SET statement.
func hasSetStatement(stmts []ast.Statement) bool {
	for _, s := range stmts {
		if _, ok := s.(*ast.SetStatement); ok {
			return true
		}
	}
	return false
}

// collectDatalines gathers the inline data records from the body's DATALINES
// statement (empty if there is none).
func collectDatalines(stmts []ast.Statement) []string {
	for _, s := range stmts {
		if dl, ok := s.(*ast.DatalinesStatement); ok {
			return dl.Lines
		}
	}
	return nil
}

// hasInputStatement reports whether the body contains an INPUT statement.
func hasInputStatement(stmts []ast.Statement) bool {
	for _, s := range stmts {
		if _, ok := s.(*ast.InputStatement); ok {
			return true
		}
	}
	return false
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

package runtime

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/formats"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

// Automatic PDV variables that exist during execution but are not written to the
// output dataset.
var automaticVars = map[string]bool{"_n_": true, "_error_": true}

// isByFlagVar reports whether a PDV name is an automatic BY-group flag
// (first.<var>/last.<var>), which is never written to output.
func isByFlagVar(name string) bool {
	return strings.HasPrefix(name, "first.") || strings.HasPrefix(name, "last.")
}

// flow signals how statement execution should proceed within an iteration.
type flow int

const (
	flowNormal flow = iota // continue to the next statement
	flowDelete             // abandon this row: skip remaining statements and any output
)

// dataStep holds the per-execution state of one DATA step.
type dataStep struct {
	lib       *table.Library
	pdv       *PDV
	logger    *log.Logger           // step logger (PUT without FILE writes here)
	outputs   []*table.Dataset      // datasets this step writes
	outOpts   []*ast.DatasetOptions // dataset options per output (aligned to outputs)
	explicit  bool                  // step contains at least one OUTPUT statement
	n         int                   // current iteration (_N_)
	records   []string              // data lines (datalines or infile file contents)
	recPtr    int                   // next data record to read
	infile    *ast.InfileStatement  // external flat-file record source (nil = datalines)
	file      *ast.FileStatement    // external flat-file PUT destination (nil = log)
	putLines  []string              // lines accumulated by PUT for the FILE destination
	setRows   []sourceRow           // rows from SET input datasets (concatenated)
	setPtr    int                   // next SET row to read
	keep      map[string]bool       // if non-nil, only these vars are output (lowercased)
	drop      map[string]bool       // these vars are excluded from output (lowercased)
	wheres    []ast.Expression      // WHERE conditions applied at read time
	byVars    []string              // BY variables (DATA step BY-group processing)
	byFlags   []ByFlags             // per-set-row first./last. flags (aligned to setRows)
	mergeRows []table.Row           // precomputed combined rows for MERGE
	mergePtr  int                   // next merge row to emit
	inVars    map[string]bool       // in= flag variable names (lowercased), excluded from output
	formats   map[string]string     // variable (lowercased) -> display format
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
//
// A non-nil logger receives the standard post-step NOTE for each output dataset.
func RunDataStep(ds *ast.DataStep, lib *table.Library, logger *log.Logger) error {
	d := &dataStep{lib: lib, pdv: NewPDV(), logger: logger}

	// Resolve output datasets. An unnamed DATA step writes to WORK.DATA1 in SAS;
	// we mirror that default. `data _null_;` declares no output (it runs for its
	// side effects, e.g. PUT to a file) — it is skipped here.
	if len(ds.Outputs) == 0 {
		d.outputs = append(d.outputs, table.NewDataset("", "DATA1"))
		d.outOpts = append(d.outOpts, nil)
	} else {
		for _, ref := range ds.Outputs {
			if strings.EqualFold(datasetName(ref.Name), "_null_") {
				continue
			}
			d.outputs = append(d.outputs, table.NewDataset("", datasetName(ref.Name)))
			d.outOpts = append(d.outOpts, ref.Options)
		}
	}

	d.explicit = containsOutput(ds.Body)
	d.file = findFile(ds.Body)
	d.infile = findInfile(ds.Body)
	if d.infile != nil {
		recs, err := readInfile(d.infile)
		if err != nil {
			return err
		}
		d.records = recs
	} else {
		d.records = collectDatalines(ds.Body)
	}
	d.keep, d.drop = collectKeepDrop(ds.Body)
	d.formats = collectFormats(ds.Body)
	d.wheres = collectWheres(ds.Body)
	d.defineArrays(ds.Body)
	if err := d.initRetained(ds.Body); err != nil {
		return err
	}
	hasInput := hasInputStatement(ds.Body)

	if mrg := findMerge(ds.Body); mrg != nil {
		// Match-merge: one iteration per precomputed combined row.
		if err := d.buildMerge(mrg, byVarsOf(ds.Body)); err != nil {
			return err
		}
		for d.mergePtr < len(d.mergeRows) {
			start := d.mergePtr
			if err := d.runIteration(ds.Body); err != nil {
				return err
			}
			if d.mergePtr == start {
				d.mergePtr++
			}
		}
	} else if hasSetStatement(ds.Body) {
		// Dataset-driven loop: one iteration per input row across all SET datasets.
		setRows, err := d.collectSetRows(ds.Body)
		if err != nil {
			return err
		}
		d.setRows = setRows
		d.setupByGroups(ds.Body)
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

	for i, out := range d.outputs {
		final := out
		if !d.outOpts[i].IsEmpty() {
			view, err := applyDatasetOptions(out, d.outOpts[i])
			if err != nil {
				return err
			}
			view.Lib, view.Name = out.Lib, out.Name
			final = view
		}
		d.lib.Put(final)
		logger.DatasetNote(final.Lib, final.Name, final.NObs(), len(final.Columns))
	}

	if d.file != nil {
		if err := writeFileOutput(d.file, d.putLines); err != nil {
			return err
		}
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
		return d.applyWhere()
	case *ast.DatalinesStatement:
		// The data is collected up front; the statement itself does nothing.
		return flowNormal, nil
	case *ast.InfileStatement:
		// The file is read up front; the statement itself does nothing.
		return flowNormal, nil
	case *ast.FileStatement:
		// The destination is resolved up front; the statement itself does nothing.
		return flowNormal, nil
	case *ast.PutStatement:
		line := d.buildPutLine(st)
		if d.file != nil {
			d.putLines = append(d.putLines, line)
		} else {
			d.logger.Put(line)
		}
		return flowNormal, nil
	case *ast.SetStatement:
		if d.setPtr >= len(d.setRows) {
			return flowDelete, nil
		}
		d.applySet(d.setRows[d.setPtr])
		d.applyByFlags(d.setPtr)
		d.setPtr++
		return d.applyWhere()
	case *ast.MergeStatement:
		if d.mergePtr >= len(d.mergeRows) {
			return flowDelete, nil
		}
		for name, v := range d.mergeRows[d.mergePtr] {
			d.pdv.Set(name, v)
		}
		d.mergePtr++
		return d.applyWhere()
	case *ast.WhereStatement:
		// WHERE is collected up front and applied at read time; nothing to do here.
		return flowNormal, nil
	case *ast.RetainStatement:
		// Retention/initialization is handled once at step setup (initRetained).
		return flowNormal, nil
	case *ast.ArrayStatement:
		// Array definitions are registered at setup (defineArrays); no-op here.
		return flowNormal, nil
	case *ast.ArrayElementAssignment:
		idx, err := Eval(st.Index, d.pdv)
		if err != nil {
			return flowNormal, err
		}
		name, ok := d.pdv.ArrayElement(st.Name, int(idx.Num))
		if !ok {
			return flowNormal, fmt.Errorf("array subscript out of range: %s{%v}", st.Name, idx.Display())
		}
		v, err := Eval(st.Value, d.pdv)
		if err != nil {
			return flowNormal, err
		}
		d.pdv.Set(name, v)
		return flowNormal, nil
	case *ast.SumStatement:
		add, err := Eval(st.Expr, d.pdv)
		if err != nil {
			return flowNormal, err
		}
		cur := d.pdv.Get(st.Var)
		base := 0.0
		if !cur.IsMissing() {
			base = cur.Num
		}
		if !add.IsMissing() {
			base += add.Num
		}
		d.pdv.Set(st.Var, table.Num(base))
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
	case *ast.DoStatement:
		return d.execDo(st)
	default:
		// Unsupported in this phase; skip.
		return flowNormal, nil
	}
}

// execDo executes a DO...END block in its four forms, propagating a flowDelete
// from the body out of the loop (a deleted row abandons the whole iteration).
func (d *dataStep) execDo(st *ast.DoStatement) (flow, error) {
	switch st.Kind {
	case ast.DoSimple:
		return d.execStatements(st.Body)

	case ast.DoIterative:
		return d.execDoIterative(st)

	case ast.DoWhile:
		for {
			cond, err := Eval(st.Cond, d.pdv)
			if err != nil {
				return flowNormal, err
			}
			if !truthy(cond) {
				return flowNormal, nil
			}
			f, err := d.execStatements(st.Body)
			if err != nil || f == flowDelete {
				return f, err
			}
		}

	case ast.DoUntil:
		for {
			f, err := d.execStatements(st.Body)
			if err != nil || f == flowDelete {
				return f, err
			}
			cond, err := Eval(st.Cond, d.pdv)
			if err != nil {
				return flowNormal, err
			}
			if truthy(cond) {
				return flowNormal, nil
			}
		}
	}
	return flowNormal, nil
}

// execDoIterative runs `do var = from to to [by by]`. The loop variable is left
// at the first value past the bound (matching SAS, where `do i=1 to 3` leaves
// i=4). Missing or zero step bounds skip the loop to avoid non-termination.
func (d *dataStep) execDoIterative(st *ast.DoStatement) (flow, error) {
	from, err := Eval(st.From, d.pdv)
	if err != nil {
		return flowNormal, err
	}
	to, err := Eval(st.To, d.pdv)
	if err != nil {
		return flowNormal, err
	}
	by := 1.0
	if st.By != nil {
		byVal, err := Eval(st.By, d.pdv)
		if err != nil {
			return flowNormal, err
		}
		by = byVal.Num
	}
	if from.IsMissing() || to.IsMissing() || by == 0 {
		return flowNormal, nil
	}

	v := from.Num
	for (by > 0 && v <= to.Num) || (by < 0 && v >= to.Num) {
		d.pdv.Set(st.Var, table.Num(v))
		f, err := d.execStatements(st.Body)
		if err != nil || f == flowDelete {
			return f, err
		}
		v += by
	}
	d.pdv.Set(st.Var, table.Num(v)) // terminal value past the bound
	return flowNormal, nil
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
		ln := strings.ToLower(name)
		if automaticVars[ln] || isByFlagVar(ln) || d.inVars[ln] || d.drop[ln] || (d.keep != nil && !d.keep[ln]) {
			continue
		}
		v := d.pdv.Get(name)
		ds.AddColumn(table.Column{Name: name, Kind: v.Kind, Format: d.formats[ln]})
		row[ln] = v
	}
	ds.AppendRow(row)
}

// applyInput parses a data record using list input (whitespace-delimited fields,
// matched positionally to the INPUT variable list) and stores the values in the
// PDV. A `$` variable is character; otherwise the field is parsed as a number.
// Missing fields, "." , and unparseable numbers become the typed missing value.
func (d *dataStep) applyInput(st *ast.InputStatement, line string) {
	fields := d.splitFields(line)
	for i, v := range st.Vars {
		var val table.Value
		switch {
		case i >= len(fields):
			if v.Char {
				val = table.MissingChar()
			} else {
				val = table.MissingNum()
			}
		case v.Informat != "":
			val = formats.ParseInput(fields[i], v.Informat)
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

// setupByGroups, when the step has a BY statement alongside SET, computes the
// first./last. flags over the (assumed BY-sorted) input rows.
func (d *dataStep) setupByGroups(stmts []ast.Statement) {
	for _, s := range stmts {
		by, ok := s.(*ast.ByStatement)
		if !ok {
			continue
		}
		d.byVars = by.Vars
		if len(d.setRows) == 0 {
			return
		}
		tmp := table.NewDataset("", "_by_")
		tmp.Columns = d.setRows[0].ds.Columns
		for _, sr := range d.setRows {
			tmp.Rows = append(tmp.Rows, sr.row)
		}
		d.byFlags = ComputeByGroups(tmp, d.byVars)
		return
	}
}

// applyByFlags sets the automatic first.<var>/last.<var> variables in the PDV
// for the input row at index i.
func (d *dataStep) applyByFlags(i int) {
	if d.byFlags == nil || i >= len(d.byFlags) {
		return
	}
	f := d.byFlags[i]
	for k, v := range d.byVars {
		d.pdv.Set("first."+v, boolNum(f.First[k]))
		d.pdv.Set("last."+v, boolNum(f.Last[k]))
	}
}

func boolNum(b bool) table.Value {
	if b {
		return table.Num(1)
	}
	return table.Num(0)
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
func (d *dataStep) collectSetRows(stmts []ast.Statement) ([]sourceRow, error) {
	var rows []sourceRow
	for _, s := range stmts {
		set, ok := s.(*ast.SetStatement)
		if !ok {
			continue
		}
		for _, ref := range set.Refs {
			src, found, err := d.lib.Resolve(ref.Name)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			view, err := applyDatasetOptions(src, ref.Options)
			if err != nil {
				return nil, err
			}
			for _, r := range view.Rows {
				rows = append(rows, sourceRow{row: r, ds: view})
			}
		}
	}
	return rows, nil
}

// defineArrays registers every ARRAY statement's element list in the PDV and
// declares the element variables (numeric by default) so subscripted access and
// output column order work.
func (d *dataStep) defineArrays(stmts []ast.Statement) {
	for _, s := range stmts {
		if arr, ok := s.(*ast.ArrayStatement); ok {
			d.pdv.DefineArray(arr.Name, arr.Elements)
			for _, e := range arr.Elements {
				d.pdv.Declare(e, table.Numeric)
			}
		}
	}
}

// initRetained scans the body for RETAIN and sum statements, marks those
// variables retained in the PDV, and sets their initial values once before the
// implicit loop: a RETAIN initial (evaluated), else missing; a sum variable is
// initialized to 0.
func (d *dataStep) initRetained(stmts []ast.Statement) error {
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.RetainStatement:
			for _, name := range st.Vars {
				d.pdv.Retain(name)
				if expr, ok := st.Initials[strings.ToLower(name)]; ok {
					v, err := Eval(expr, d.pdv)
					if err != nil {
						return err
					}
					d.pdv.Set(name, v)
				} else if !d.pdv.Has(name) {
					d.pdv.Declare(name, table.Numeric)
				}
			}
		case *ast.SumStatement:
			d.pdv.Retain(st.Var)
			if !d.pdv.Has(st.Var) {
				d.pdv.Set(st.Var, table.Num(0))
			}
		}
	}
	return nil
}

// applyWhere evaluates the step's WHERE conditions against the just-read row.
// If any is false the row is dropped (flowDelete), matching SAS's read-time
// filtering. With no WHERE conditions it is a no-op.
func (d *dataStep) applyWhere() (flow, error) {
	for _, cond := range d.wheres {
		v, err := Eval(cond, d.pdv)
		if err != nil {
			return flowNormal, err
		}
		if !truthy(v) {
			return flowDelete, nil
		}
	}
	return flowNormal, nil
}

// collectFormats merges all FORMAT statements into a var→format map.
func collectFormats(stmts []ast.Statement) map[string]string {
	out := map[string]string{}
	for _, s := range stmts {
		if f, ok := s.(*ast.FormatStatement); ok {
			for k, v := range f.Formats {
				out[k] = v
			}
		}
	}
	return out
}

// collectWheres gathers the WHERE conditions from the body.
func collectWheres(stmts []ast.Statement) []ast.Expression {
	var out []ast.Expression
	for _, s := range stmts {
		if w, ok := s.(*ast.WhereStatement); ok {
			out = append(out, w.Condition)
		}
	}
	return out
}

// collectKeepDrop scans the body for KEEP and DROP statements and returns the
// resulting variable filters (lowercased). keep is nil unless a KEEP statement
// appears (nil means "keep all"); drop is always a set. Multiple statements
// accumulate.
func collectKeepDrop(stmts []ast.Statement) (keep, drop map[string]bool) {
	drop = make(map[string]bool)
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.KeepStatement:
			if keep == nil {
				keep = make(map[string]bool)
			}
			for _, v := range st.Vars {
				keep[strings.ToLower(v)] = true
			}
		case *ast.DropStatement:
			for _, v := range st.Vars {
				drop[strings.ToLower(v)] = true
			}
		}
	}
	return keep, drop
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

// findInfile returns the INFILE statement in the body, or nil if none.
func findInfile(stmts []ast.Statement) *ast.InfileStatement {
	for _, s := range stmts {
		if in, ok := s.(*ast.InfileStatement); ok {
			return in
		}
	}
	return nil
}

// findFile returns the FILE statement in the body, or nil if none.
func findFile(stmts []ast.Statement) *ast.FileStatement {
	for _, s := range stmts {
		if f, ok := s.(*ast.FileStatement); ok {
			return f
		}
	}
	return nil
}

// writeFileOutput writes the accumulated PUT lines to the FILE destination, one
// per line with a trailing newline. A FILE with no PUT output still produces an
// (empty) file, matching SAS.
func writeFileOutput(f *ast.FileStatement, lines []string) error {
	var b strings.Builder
	for _, ln := range lines {
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(f.Path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("file %q: %w", f.Path, err)
	}
	return nil
}

// buildPutLine renders one PUT statement to a line. Items are joined by the
// FILE's delimiter — a single blank for default list output, the DLM= value, or
// a comma under DSD. Variable values are rendered with their inline or
// associated format; under DSD, values containing the delimiter or a quote are
// wrapped in double quotes (embedded quotes doubled).
func (d *dataStep) buildPutLine(st *ast.PutStatement) string {
	sep := " "
	dsd := false
	if d.file != nil {
		dsd = d.file.DSD
		switch {
		case d.file.Delimiter != "":
			sep = d.file.Delimiter
		case dsd:
			sep = ","
		}
	}
	parts := make([]string, 0, len(st.Items))
	for _, it := range st.Items {
		if it.IsLiteral {
			parts = append(parts, it.Literal)
			continue
		}
		format := it.Format
		if format == "" {
			format = d.formats[strings.ToLower(it.Var)]
		}
		s := strings.TrimSpace(formats.Apply(d.pdv.Get(it.Var), format))
		if dsd {
			s = dsdQuote(s, sep)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, sep)
}

// dsdQuote wraps s in double quotes (doubling embedded quotes) when it contains
// the delimiter, a quote, or a newline — the DSD output convention.
func dsdQuote(s, sep string) string {
	if strings.Contains(s, sep) || strings.Contains(s, "\"") || strings.Contains(s, "\n") {
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}
	return s
}

// readInfile reads the external flat file named by an INFILE statement into one
// record per line, applying FIRSTOBS=/OBS= line bounds (1-based; 0 = unset).
// CRLF is normalized to LF and a single trailing newline is dropped.
func readInfile(in *ast.InfileStatement) ([]string, error) {
	data, err := os.ReadFile(in.Path)
	if err != nil {
		return nil, fmt.Errorf("infile %q: %w", in.Path, err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil, nil
	}
	lines := strings.Split(text, "\n")
	first := in.Firstobs
	if first < 1 {
		first = 1
	}
	if first > len(lines) {
		return nil, nil
	}
	last := len(lines)
	if in.Obs > 0 && in.Obs < last {
		last = in.Obs
	}
	return lines[first-1 : last], nil
}

// splitFields breaks an input record into fields according to the active INFILE
// delimiter settings: whitespace list input by default, a fixed delimiter with
// DLM=, and CSV-style quoted/missing-aware parsing with DSD.
func (d *dataStep) splitFields(line string) []string {
	if d.infile == nil {
		return strings.Fields(line)
	}
	delim := d.infile.Delimiter
	if delim == "" && d.infile.DSD {
		delim = ","
	}
	if delim == "" {
		return strings.Fields(line)
	}
	sep := rune(delim[0])
	if d.infile.DSD {
		return splitDSD(line, sep)
	}
	// DLM without DSD: consecutive delimiters collapse to one (SAS behavior).
	return strings.FieldsFunc(line, func(r rune) bool { return r == sep })
}

// splitDSD parses one delimited line with DSD semantics: fields may be wrapped
// in double quotes (a delimiter inside quotes is literal; "" is an escaped
// quote), and consecutive delimiters yield empty (missing) fields.
func splitDSD(line string, sep rune) []string {
	var fields []string
	var b strings.Builder
	inQuotes := false
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case inQuotes:
			if r == '"' {
				if i+1 < len(runes) && runes[i+1] == '"' {
					b.WriteRune('"')
					i++
				} else {
					inQuotes = false
				}
			} else {
				b.WriteRune(r)
			}
		case r == '"':
			inQuotes = true
		case r == sep:
			fields = append(fields, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	fields = append(fields, b.String())
	return fields
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

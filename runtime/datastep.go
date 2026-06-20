package runtime

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/flatfile"
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
	logger    *log.Logger             // step logger (PUT without FILE writes here)
	outputs   []*table.Dataset        // datasets this step writes
	outOpts   []*ast.DatasetOptions   // dataset options per output (aligned to outputs)
	outNames  []string                // full (possibly libref-qualified) output names, aligned to outputs
	explicit  bool                    // step contains at least one OUTPUT statement
	n         int                     // current iteration (_N_)
	records   []string                // data lines (datalines or infile file contents)
	recPtr    int                     // next data record to read
	infile    *ast.InfileStatement    // external flat-file record source (nil = datalines)
	file      *ast.FileStatement      // external flat-file PUT destination (nil = log)
	putLines  []string                // lines accumulated by PUT for the FILE destination
	setRows   []sourceRow             // rows from SET input datasets (concatenated)
	setPtr    int                     // next SET row to read
	keep      map[string]bool         // if non-nil, only these vars are output (lowercased)
	drop      map[string]bool         // these vars are excluded from output (lowercased)
	wheres    []ast.Expression        // WHERE conditions applied at read time
	byVars    []string                // BY variables (DATA step BY-group processing)
	byFlags   []ByFlags               // per-set-row first./last. flags (aligned to setRows)
	mergeRows []table.Row             // precomputed combined rows for MERGE
	mergePtr  int                     // next merge row to emit
	inVars    map[string]bool         // in= flag variable names (lowercased), excluded from output
	formats   map[string]string       // variable (lowercased) -> display format
	labels    map[string]string       // variable (lowercased) -> descriptive label
	srcCols   map[string]table.Column // variable (lowercased) -> source column metadata (SET/MERGE), first source wins
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
		d.outNames = append(d.outNames, "")
	} else {
		for _, ref := range ds.Outputs {
			if strings.EqualFold(datasetName(ref.Name), "_null_") {
				continue
			}
			d.outputs = append(d.outputs, table.NewDataset("", datasetName(ref.Name)))
			d.outOpts = append(d.outOpts, ref.Options)
			d.outNames = append(d.outNames, ref.Name)
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
	d.labels = collectLabels(ds.Body)
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
		handled, err := d.lib.StoreExternal(d.outNames[i], final)
		if err != nil {
			return err
		}
		if !handled {
			d.lib.Put(final)
		}
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
		col := table.Column{Name: name, Kind: v.Kind, Format: d.formats[ln], Label: d.labels[ln]}
		// Carry attributes from the SET/MERGE source variable. An explicit FORMAT or
		// LABEL statement in this step wins; otherwise the variable keeps the
		// source's. Informat/length always carry through (no per-step override yet).
		if src, ok := d.srcCols[ln]; ok {
			if col.Format == "" {
				col.Format = src.Format
			}
			if col.Label == "" {
				col.Label = src.Label
			}
			col.Informat = src.Informat
			col.Length = src.Length
		}
		ds.AddColumn(col)
		row[ln] = v
	}
	ds.AppendRow(row)
}

// recordSourceCols captures a SET/MERGE source dataset's column metadata so the
// output dataset can inherit each variable's format, informat, label, and length
// (SAS attribute inheritance). The first source to define a variable wins, which
// matches SET (statement order) and MERGE (left-to-right) attribute resolution.
func (d *dataStep) recordSourceCols(ds *table.Dataset) {
	if d.srcCols == nil {
		d.srcCols = make(map[string]table.Column)
	}
	for _, c := range ds.Columns {
		ln := strings.ToLower(c.Name)
		if _, seen := d.srcCols[ln]; !seen {
			d.srcCols[ln] = c
		}
	}
}

// applyInput parses a data record using list input (whitespace-delimited fields,
// matched positionally to the INPUT variable list) and stores the values in the
// PDV. A `$` variable is character; otherwise the field is parsed as a number.
// Missing fields, "." , and unparseable numbers become the typed missing value.
func (d *dataStep) applyInput(st *ast.InputStatement, line string) {
	if columnMode(st) {
		d.applyColumnInput(st, line)
		return
	}
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

// columnMode reports whether an INPUT statement uses column/pointer input
// (explicit column ranges or `@n`/`+n` pointers) rather than delimited list
// input. In that mode fields are read by character position, not by splitting on
// a delimiter.
func columnMode(st *ast.InputStatement) bool {
	for _, v := range st.Vars {
		if v.ColStart > 0 || v.At > 0 || v.Plus > 0 {
			return true
		}
	}
	return false
}

// applyColumnInput reads each variable from a fixed character position in the
// record. A 1-based column pointer advances as fields are read: `@n` sets it
// absolutely, `+n` skips forward, an explicit `start-end` range reads that span,
// an informat reads its width from the pointer, and a bare variable scans a
// whitespace-delimited token from the pointer.
func (d *dataStep) applyColumnInput(st *ast.InputStatement, line string) {
	col := 1 // 1-based next read column
	for _, v := range st.Vars {
		if v.At > 0 {
			col = v.At
		}
		if v.Plus > 0 {
			col += v.Plus
		}

		var field string
		switch {
		case v.ColStart > 0:
			start := v.ColStart
			end := v.ColEnd
			if end == 0 {
				end = start
			}
			field = colSlice(line, start, end)
			col = end + 1
		case v.Informat != "":
			if w := informatWidth(v.Informat); w > 0 {
				field = colSlice(line, col, col+w-1)
				col += w
			} else { // no explicit width — scan a token
				field, col = scanToken(line, col)
			}
		default:
			field, col = scanToken(line, col)
		}

		var val table.Value
		switch {
		case v.Informat != "":
			val = formats.ParseInput(field, v.Informat)
		case v.Char:
			val = table.Char(strings.TrimRight(field, " "))
		default:
			val = parseNum(strings.TrimSpace(field))
		}
		d.pdv.Set(v.Name, val)
	}
}

// colSlice returns the 1-based inclusive [start,end] substring of line, clamped
// to the line's bounds (a range past the end yields the empty string).
func colSlice(line string, start, end int) string {
	if start < 1 {
		start = 1
	}
	if start > len(line) {
		return ""
	}
	if end > len(line) {
		end = len(line)
	}
	if end < start {
		return ""
	}
	return line[start-1 : end]
}

// scanToken reads a whitespace-delimited token starting at or after the 1-based
// column, returning the token and the 1-based column just past it.
func scanToken(line string, col int) (string, int) {
	i := col - 1
	if i < 0 {
		i = 0
	}
	for i < len(line) && line[i] == ' ' {
		i++
	}
	start := i
	for i < len(line) && line[i] != ' ' {
		i++
	}
	return line[start:i], i + 1
}

// informatWidth returns the field width declared by an informat spec (e.g.
// "$10." -> 10, "comma8.2" -> 8), or 0 when none is given.
func informatWidth(informat string) int {
	s := strings.TrimPrefix(informat, "$")
	// width is the run of digits before the dot.
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		dot = len(s)
	}
	j := dot
	for j > 0 && s[j-1] >= '0' && s[j-1] <= '9' {
		j--
	}
	w, _ := strconv.Atoi(s[j:dot])
	return w
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
			d.recordSourceCols(view)
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

// collectLabels merges all LABEL statements into a var→label map.
func collectLabels(stmts []ast.Statement) map[string]string {
	out := map[string]string{}
	for _, s := range stmts {
		if l, ok := s.(*ast.LabelStatement); ok {
			for k, v := range l.Labels {
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
	if err := flatfile.WriteLines(f.Path, lines); err != nil {
		return fmt.Errorf("file %w", err)
	}
	return nil
}

// buildPutLine renders one PUT statement to a line. Items are joined by the
// FILE's delimiter — a single blank for default list output, the DLM= value, or
// a comma under DSD. Variable values are rendered with their inline or
// associated format; under DSD, values containing the delimiter or a quote are
// wrapped in double quotes (embedded quotes doubled).
func (d *dataStep) buildPutLine(st *ast.PutStatement) string {
	if putColumnMode(st) {
		return d.buildPutColumnLine(st)
	}
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
			s = flatfile.Quote(s, sep)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, sep)
}

// putColumnMode reports whether a PUT statement uses column/pointer output
// (explicit column ranges or `@n`/`+n` pointers) rather than delimiter-joined
// list output.
func putColumnMode(st *ast.PutStatement) bool {
	for _, it := range st.Items {
		if it.ColStart > 0 || it.At > 0 || it.Plus > 0 {
			return true
		}
	}
	return false
}

// buildPutColumnLine renders a PUT statement that positions its items by column.
// A 1-based cursor advances as items are written: `@n` sets it absolutely, `+n`
// skips forward, an explicit `start-end` range writes into that span (character
// left-justified, numeric right-justified within the width), and any other item
// is written starting at the cursor. Trailing blanks are trimmed from the line.
func (d *dataStep) buildPutColumnLine(st *ast.PutStatement) string {
	var buf []byte
	col := 1 // 1-based next write column
	place := func(text string, start, width int) {
		// Pad the buffer up to the write span.
		end := start + len(text) - 1
		if width > 0 {
			end = start + width - 1
		}
		for len(buf) < end {
			buf = append(buf, ' ')
		}
		for k, ch := range []byte(text) {
			if start-1+k < len(buf) {
				buf[start-1+k] = ch
			}
		}
	}
	for _, it := range st.Items {
		if it.At > 0 {
			col = it.At
		}
		if it.Plus > 0 {
			col += it.Plus
		}

		var text string
		if it.IsLiteral {
			text = it.Literal
		} else {
			format := it.Format
			if format == "" {
				format = d.formats[strings.ToLower(it.Var)]
			}
			v := d.pdv.Get(it.Var)
			text = strings.TrimSpace(formats.Apply(v, format))
			if it.ColStart > 0 {
				width := 1
				if it.ColEnd > 0 {
					width = it.ColEnd - it.ColStart + 1
				}
				text = justify(text, width, v.Kind == table.Character)
			}
		}

		switch {
		case it.ColStart > 0:
			start := it.ColStart
			width := 1
			if it.ColEnd > 0 {
				width = it.ColEnd - it.ColStart + 1
			}
			place(text, start, width)
			col = start + width
		default:
			place(text, col, 0)
			col += len(text)
		}
	}
	return strings.TrimRight(string(buf), " ")
}

// justify fits text into a fixed width: character values are left-justified,
// numeric values right-justified; either is truncated if too long.
func justify(text string, width int, char bool) string {
	if len(text) >= width {
		return text[:width]
	}
	pad := strings.Repeat(" ", width-len(text))
	if char {
		return text + pad
	}
	return pad + text
}

// readInfile reads the external flat file named by an INFILE statement into one
// record per line, applying FIRSTOBS=/OBS= line bounds.
func readInfile(in *ast.InfileStatement) ([]string, error) {
	lines, err := flatfile.ReadLines(in.Path, in.Firstobs, in.Obs)
	if err != nil {
		return nil, fmt.Errorf("infile %w", err)
	}
	return lines, nil
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
	return flatfile.SplitDelim(line, rune(delim[0]), d.infile.DSD)
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

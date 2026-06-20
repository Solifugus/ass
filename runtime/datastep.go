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
	recCursor int                     // 1-based column in the current record where the next list read resumes (line-hold)
	holdCross bool                    // a trailing `@@` is holding the current line across iterations
	holdLine  bool                    // a trailing `@` is holding the current line within the iteration
	mlHold    bool                    // a trailing hold is active on a `#n` multi-line record
	mlBase    int                     // held base record index for the multi-line hold
	mlCursors map[int]int             // per-line-offset 1-based cursor for the held multi-line record
	infile    *ast.InfileStatement    // external flat-file record source (nil = datalines)
	file      *ast.FileStatement      // external flat-file PUT destination (nil = log)
	putLines  []string                // lines accumulated by PUT for the FILE destination
	putBuf    string                  // held partial output line (trailing `@`/`@@` on PUT)
	putHeld   bool                    // a trailing `@`/`@@` is holding the output line
	putHoldX  bool                    // the output hold is `@@` (persists across iterations)
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
			// A single `@` holds the line only within the iteration; release it at
			// the iteration boundary so the next iteration reads a fresh record.
			if d.holdLine && !d.holdCross {
				if d.mlHold { // multi-line `@`: advance past the whole record group
					d.recPtr = d.mlBase + 1
					d.mlHold = false
					d.mlCursors = nil
				} else {
					d.recPtr++
				}
				d.holdLine = false
				d.recCursor = 1
			}
			if d.recPtr == start && !d.holdCross { // ensure progress unless `@@` holds the line
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

	// Release any output line still held by a trailing `@@` at end of step.
	d.flushPutHold()

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
	// A single trailing `@` holds the output line only within the iteration; SAS
	// writes it automatically at the iteration boundary. `@@` persists across
	// iterations, so it is left held.
	if d.putHeld && !d.putHoldX {
		d.flushPutHold()
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
		return d.execInput(st)
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
		d.execPut(st)
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
// execInput executes one INPUT statement, honoring trailing line-hold modifiers
// (`@@` across iterations, `@` within the iteration). Without a hold it consumes
// the current record and advances to the next, exactly as before. While a hold is
// active (set by this statement or a previous one), reads resume from a 1-based
// cursor into the current line so several observations can be read from one
// physical record; the record advances only when the held line is exhausted
// (`@@`), at the end of the iteration (`@`), or immediately (no hold).
func (d *dataStep) execInput(st *ast.InputStatement) (flow, error) {
	// A `#n` line pointer reads one observation across several physical records.
	if hasLinePointer(st) {
		// With a trailing `@`/`@@` hold (or while one is active), the multi-line
		// record is held across reads with per-line cursors; otherwise the record
		// advances by the number of lines consumed.
		if st.TrailingAt > 0 || d.mlHold {
			return d.execMultiLineHeld(st)
		}
		if d.recPtr >= len(d.records) {
			return flowDelete, nil // records exhausted
		}
		base := d.recPtr
		consumed := d.applyMultiLineInput(st, base)
		d.recPtr = base + consumed
		d.holdCross, d.holdLine = false, false
		d.recCursor = 1
		return d.applyWhere()
	}
	holding := d.holdCross || d.holdLine
	if !holding {
		d.recCursor = 1
	}
	// While holding across iterations, skip any held lines whose remaining content
	// has no more tokens (move on to the next physical record).
	if d.holdCross {
		for d.recPtr < len(d.records) && noMoreTokens(d.records[d.recPtr], d.recCursor) {
			d.recPtr++
			d.recCursor = 1
		}
	}
	if d.recPtr >= len(d.records) {
		return flowDelete, nil // records exhausted
	}
	line := d.records[d.recPtr]

	if holding || st.TrailingAt > 0 {
		// Cursor-based read so consecutive reads share the physical line.
		if columnMode(st) {
			d.recCursor = d.applyColumnInputFrom(st, line, d.recCursor)
		} else {
			d.recCursor = d.applyListInputFrom(st, line, d.recCursor)
		}
	} else {
		d.applyInput(st, line)
	}

	switch st.TrailingAt {
	case 2: // @@ — hold the line across iterations
		d.holdCross, d.holdLine = true, false
	case 1: // @ — hold the line for the rest of this iteration
		d.holdLine = true
	default: // no hold — release and advance to the next record
		d.holdCross, d.holdLine = false, false
		d.recPtr++
		d.recCursor = 1
	}
	return d.applyWhere()
}

// noMoreTokens reports whether the line has no further whitespace-delimited token
// at or after the 1-based cursor (only trailing blanks remain).
func noMoreTokens(line string, cursor int) bool {
	tok, _ := scanToken(line, cursor)
	return tok == ""
}

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

// applyListInputFrom reads list-input variables starting at the 1-based cursor
// into the line (whitespace-delimited), returning the cursor just past the last
// token read. Used for line-hold (`@`/`@@`) reads, where consecutive INPUTs share
// one physical record.
func (d *dataStep) applyListInputFrom(st *ast.InputStatement, line string, cursor int) int {
	col := cursor
	for _, v := range st.Vars {
		var val table.Value
		val, col = readListField(line, col, v)
		d.pdv.Set(v.Name, val)
	}
	return col
}

// readListField reads one whitespace-delimited list-input field for v starting at
// the 1-based column, returning the typed value and the column just past it.
func readListField(line string, col int, v ast.InputVar) (table.Value, int) {
	field, next := scanToken(line, col)
	var val table.Value
	switch {
	case field == "":
		if v.Char {
			val = table.MissingChar()
		} else {
			val = table.MissingNum()
		}
	case v.Informat != "":
		val = formats.ParseInput(field, v.Informat)
	case v.Char:
		val = table.Char(field)
	default:
		val = parseNum(field)
	}
	return val, next
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
	d.applyColumnInputFrom(st, line, 1)
}

// applyColumnInputFrom is applyColumnInput starting at a given 1-based column
// (for line-hold reads) and returning the column just past the last field read.
func (d *dataStep) applyColumnInputFrom(st *ast.InputStatement, line string, startCol int) int {
	col := startCol // 1-based next read column
	for _, v := range st.Vars {
		if v.At > 0 {
			col = v.At
		}
		if v.Plus > 0 {
			col += v.Plus
		}
		var val table.Value
		val, col = readColumnField(line, col, v)
		d.pdv.Set(v.Name, val)
	}
	return col
}

// readColumnField reads one column/pointer-input field for v from line at the
// 1-based column, returning the typed value and the column just past it. (The
// caller applies any `@n`/`+n` adjustment to col before calling.)
func readColumnField(line string, col int, v ast.InputVar) (table.Value, int) {
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
	return val, col
}

// hasLinePointer reports whether an INPUT statement uses a `#n` line pointer, so
// one observation is read across multiple physical records.
func hasLinePointer(st *ast.InputStatement) bool {
	for _, v := range st.Vars {
		if v.Line > 0 {
			return true
		}
	}
	return false
}

// applyMultiLineInput reads an INPUT statement that uses `#n` line pointers. The
// logical record begins at the 1-based-relative line `base` (a record index);
// `#n` selects its n-th line (resetting the column pointer to 1), and a bare
// variable continues on the current line. It returns the number of physical
// lines the record consumed (the highest line referenced, at least 1) so the
// caller can advance past them.
func (d *dataStep) applyMultiLineInput(st *ast.InputStatement, base int) int {
	colMode := columnMode(st)
	lineOff := 0 // 0-based offset from base of the line currently being read
	maxLine := 1
	col := 1
	lineAt := func(off int) string {
		idx := base + off
		if idx < 0 || idx >= len(d.records) {
			return ""
		}
		return d.records[idx]
	}
	for _, v := range st.Vars {
		if v.Line > 0 {
			lineOff = v.Line - 1
			if v.Line > maxLine {
				maxLine = v.Line
			}
			col = 1
		}
		line := lineAt(lineOff)
		var val table.Value
		if colMode {
			if v.At > 0 {
				col = v.At
			}
			if v.Plus > 0 {
				col += v.Plus
			}
			val, col = readColumnField(line, col, v)
		} else {
			val, col = readListField(line, col, v)
		}
		d.pdv.Set(v.Name, val)
	}
	return maxLine
}

// maxLinePointer returns the highest `#n` line referenced by the statement (at
// least 1), i.e. the number of physical lines the logical record spans.
func maxLinePointer(st *ast.InputStatement) int {
	max := 1
	for _, v := range st.Vars {
		if v.Line > max {
			max = v.Line
		}
	}
	return max
}

// execMultiLineHeld reads a `#n` multi-line INPUT statement under a trailing
// `@`/`@@` hold. The logical record's base line is held across iterations and a
// per-line-offset cursor lets each line be re-read token by token, so several
// observations are read from one multi-line record group (the `#n` analogue of
// `input x @@;`). The group is released — and the record pointer advanced past it
// — once its first line runs out of tokens.
func (d *dataStep) execMultiLineHeld(st *ast.InputStatement) (flow, error) {
	maxLine := maxLinePointer(st)
	if !d.mlHold {
		d.mlBase = d.recPtr
		d.mlCursors = map[int]int{}
	}

	// Release an exhausted held group (its first line has no more tokens) and
	// advance to the next record group.
	if d.mlHold {
		startCur := d.mlCursors[0]
		if startCur == 0 {
			startCur = 1
		}
		base0 := ""
		if d.mlBase >= 0 && d.mlBase < len(d.records) {
			base0 = d.records[d.mlBase]
		}
		if d.mlBase+maxLine > len(d.records) || noMoreTokens(base0, startCur) {
			d.recPtr = d.mlBase + maxLine
			d.mlHold, d.holdCross, d.holdLine = false, false, false
			d.mlCursors = map[int]int{}
			if d.recPtr >= len(d.records) {
				return flowDelete, nil
			}
			d.mlBase = d.recPtr
		}
	}
	if d.mlBase >= len(d.records) {
		return flowDelete, nil
	}

	d.applyMultiLineInputCursors(st)

	switch st.TrailingAt {
	case 2: // @@ — hold across iterations
		d.mlHold, d.holdCross, d.holdLine = true, true, false
	case 1: // @ — hold for the rest of this iteration
		d.mlHold, d.holdLine, d.holdCross = true, true, false
	default: // entered via an active hold but this read has no trailing hold: release
		d.recPtr = d.mlBase + maxLine
		d.mlHold, d.holdCross, d.holdLine = false, false, false
		d.mlCursors = map[int]int{}
	}
	return d.applyWhere()
}

// applyMultiLineInputCursors reads a `#n` statement using the held per-line
// cursors (d.mlCursors, keyed by 0-based line offset from d.mlBase), advancing
// each line's cursor as tokens/fields are consumed.
func (d *dataStep) applyMultiLineInputCursors(st *ast.InputStatement) {
	colMode := columnMode(st)
	lineOff := 0
	lineAt := func(off int) string {
		idx := d.mlBase + off
		if idx < 0 || idx >= len(d.records) {
			return ""
		}
		return d.records[idx]
	}
	for _, v := range st.Vars {
		if v.Line > 0 {
			lineOff = v.Line - 1
		}
		col := d.mlCursors[lineOff]
		if col == 0 {
			col = 1
		}
		line := lineAt(lineOff)
		var val table.Value
		if colMode {
			if v.At > 0 {
				col = v.At
			}
			if v.Plus > 0 {
				col += v.Plus
			}
			val, col = readColumnField(line, col, v)
		} else {
			val, col = readListField(line, col, v)
		}
		d.mlCursors[lineOff] = col
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
			src, found, err := d.lib.ResolveFiltered(ref.Name, pushdownSelection(ref.Options))
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
// putHasLinePointer reports whether a PUT statement uses a `#n` line pointer, so
// it emits several physical output lines.
func putHasLinePointer(st *ast.PutStatement) bool {
	for _, it := range st.Items {
		if it.Line > 0 {
			return true
		}
	}
	return false
}

// buildPutLines renders a PUT statement to one or more physical output lines.
// Without a `#n` line pointer it is a single line. With `#n`, items are
// partitioned into per-line buckets (a `#n` directs subsequent items to line n),
// and lines 1..max are emitted in order — an unreferenced line is blank. Each
// line is rendered by the existing single-line logic.
func (d *dataStep) buildPutLines(st *ast.PutStatement) []string {
	if !putHasLinePointer(st) {
		return []string{d.buildPutLine(st)}
	}
	buckets := map[int][]ast.PutItem{}
	maxLine := 1
	cur := 1
	for _, it := range st.Items {
		if it.Line > 0 {
			cur = it.Line
			if cur > maxLine {
				maxLine = cur
			}
		}
		it.Line = 0 // render the item normally within its line
		buckets[cur] = append(buckets[cur], it)
	}
	lines := make([]string, maxLine)
	for n := 1; n <= maxLine; n++ {
		if items, ok := buckets[n]; ok {
			lines[n-1] = d.buildPutLine(&ast.PutStatement{Items: items})
		}
	}
	return lines
}

// execPut renders a PUT statement and routes it to the FILE destination (or the
// log), honoring a trailing `@`/`@@` output line-hold. With a hold active, the
// rendered segment continues the held physical line instead of starting a new
// one: list segments are joined by the FILE's separator, column/pointer segments
// overlay onto the held line at their absolute columns. A `@` holds the line
// within the iteration (released at the iteration boundary); `@@` holds it across
// iterations (released only by a PUT without a trailing hold, or at end of step).
func (d *dataStep) execPut(st *ast.PutStatement) {
	// `#n` multi-line PUT is not combined with an output hold: flush any held line
	// first, then emit the multi-line output as its own physical lines.
	if putHasLinePointer(st) {
		d.flushPutHold()
		d.writePut(d.buildPutLines(st))
		return
	}

	seg := d.buildPutLine(st)
	switch {
	case !d.putHeld:
		d.putBuf = seg
	case putColumnMode(st):
		d.putBuf = overlayLine(d.putBuf, seg)
	default:
		sep := d.listSep()
		if d.putBuf == "" || seg == "" {
			d.putBuf += seg
		} else {
			d.putBuf += sep + seg
		}
	}

	if st.TrailingAt > 0 {
		d.putHeld = true
		d.putHoldX = st.TrailingAt == 2
		return
	}
	d.writePut([]string{d.putBuf})
	d.putBuf, d.putHeld, d.putHoldX = "", false, false
}

// flushPutHold writes any held PUT line and clears the hold. It is a no-op when
// no line is held.
func (d *dataStep) flushPutHold() {
	if d.putHeld {
		d.writePut([]string{d.putBuf})
		d.putBuf, d.putHeld, d.putHoldX = "", false, false
	}
}

// writePut routes rendered PUT lines to the FILE buffer or, with no FILE, the log.
func (d *dataStep) writePut(lines []string) {
	if d.file != nil {
		d.putLines = append(d.putLines, lines...)
		return
	}
	for _, line := range lines {
		d.logger.Put(line)
	}
}

// listSep is the separator that joins held list-mode PUT segments: the FILE's
// DLM= value, a comma under DSD, or a single blank for default list output.
func (d *dataStep) listSep() string {
	if d.file != nil {
		switch {
		case d.file.Delimiter != "":
			return d.file.Delimiter
		case d.file.DSD:
			return ","
		}
	}
	return " "
}

// overlayLine merges a column/pointer PUT segment onto a held line: every
// non-blank byte of seg is written at its absolute column over base, padding base
// with blanks where seg is longer. Blanks in seg are padding and leave base
// intact, so a held prefix survives an `@n`-positioned continuation.
func overlayLine(base, seg string) string {
	b, s := []byte(base), []byte(seg)
	for len(b) < len(s) {
		b = append(b, ' ')
	}
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			b[i] = s[i]
		}
	}
	return string(b)
}

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
		if it.AllVars { // `_all_`: every PDV variable as name=value
			for _, name := range d.pdv.Names() {
				parts = append(parts, d.namedPart(name, "", dsd, sep))
			}
			continue
		}
		if it.IsLiteral {
			parts = append(parts, it.Literal)
			continue
		}
		if it.Named { // `x=` named output
			parts = append(parts, d.namedPart(it.Var, it.Format, dsd, sep))
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

// namedPart renders one named-output element as `name=value`, using the
// variable's inline or associated format and DSD-quoting the value when needed.
func (d *dataStep) namedPart(name, format string, dsd bool, sep string) string {
	if format == "" {
		format = d.formats[strings.ToLower(name)]
	}
	s := strings.TrimSpace(formats.Apply(d.pdv.Get(name), format))
	if dsd {
		s = flatfile.Quote(s, sep)
	}
	return name + "=" + s
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

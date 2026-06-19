package proc

import (
	"strconv"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/flatfile"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() { Register("import", importProc{}) }

// importProc implements PROC IMPORT for delimited flat files (DBMS=CSV/TAB/DLM):
//
//	proc import datafile="path" out=ds dbms=csv replace;
//	  getnames=yes;   /* first row holds column names (default) */
//	  datarow=2;      /* 1-based row where data begins */
//	  delimiter=',';  /* or dlm=, for DBMS=DLM */
//	run;
//
// It reads the file with DSD (CSV) field semantics, derives column names from the
// header (or VAR1..VARn when GETNAMES=NO), and sniffs each column's type: a
// column whose every non-empty data value parses as a number becomes numeric,
// otherwise character. The materialized dataset is stored under OUT=.
type importProc struct{}

func (importProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	var datafile, out, dbms string
	for _, o := range step.Options {
		switch strings.ToLower(o.Name) {
		case "datafile", "file":
			datafile = o.Value
		case "out":
			out = o.Value
		case "dbms":
			dbms = strings.ToLower(o.Value)
		case "replace":
			// always replace; OUT= datasets are overwritten in the library
		}
	}
	if datafile == "" {
		logger.Error("PROC IMPORT: DATAFILE= is required.")
		return nil
	}
	if out == "" {
		logger.Error("PROC IMPORT: OUT= is required.")
		return nil
	}

	opts := readImportOptions(step.Body)
	sep := delimiterFor(dbms, opts.delimiter)

	lines, err := flatfile.ReadLines(datafile, 0, 0)
	if err != nil {
		logger.Error("PROC IMPORT: %v", err)
		return nil
	}

	// Resolve the header and the first data row. GETNAMES=YES uses row 1 for
	// names; DATAROW (1-based) overrides where data starts (default: 2 with a
	// header, 1 without).
	var names []string
	dataStart := 0
	if opts.getnames {
		if len(lines) > 0 {
			names = flatfile.SplitDelim(lines[0], sep, true)
		}
		dataStart = 1
	}
	if opts.datarow > 0 {
		dataStart = opts.datarow - 1
	}
	if dataStart < 0 {
		dataStart = 0
	}

	var records [][]string
	for i := dataStart; i < len(lines); i++ {
		records = append(records, flatfile.SplitDelim(lines[i], sep, true))
	}

	ncol := len(names)
	for _, rec := range records {
		if len(rec) > ncol {
			ncol = len(rec)
		}
	}
	if !opts.getnames {
		names = make([]string, ncol)
	}
	for i := 0; i < ncol; i++ {
		if i >= len(names) || strings.TrimSpace(names[i]) == "" {
			if i >= len(names) {
				names = append(names, "")
			}
			names[i] = "VAR" + strconv.Itoa(i+1)
		} else {
			names[i] = strings.TrimSpace(names[i])
		}
	}

	ds := table.NewDataset("", datasetName(out))
	kinds := sniffKinds(records, ncol)
	for i, n := range names {
		ds.AddColumn(table.Column{Name: n, Kind: kinds[i]})
	}
	for _, rec := range records {
		row := make(table.Row)
		for i, n := range names {
			ln := strings.ToLower(n)
			field := ""
			if i < len(rec) {
				field = rec[i]
			}
			if kinds[i] == table.Numeric {
				row[ln] = parseImportNum(field)
			} else {
				row[ln] = table.Char(field)
			}
		}
		ds.AppendRow(row)
	}

	lib.Put(ds)
	logger.Note("The data set %s.%s has %d observations and %d variables.",
		strings.ToUpper(ds.Lib), strings.ToUpper(ds.Name), ds.NObs(), len(ds.Columns))
	return nil
}

// importOptions holds the PROC IMPORT body statement settings.
type importOptions struct {
	getnames  bool
	datarow   int
	delimiter string
}

// readImportOptions parses GETNAMES, DATAROW, and DELIMITER/DLM from the body's
// raw statements. GETNAMES defaults to yes.
func readImportOptions(stmts []ast.Statement) importOptions {
	opts := importOptions{getnames: true}
	for _, s := range stmts {
		raw, ok := s.(*ast.RawStatement)
		if !ok {
			continue
		}
		key, val := splitRawOption(raw.Text)
		switch key {
		case "getnames":
			opts.getnames = !isNo(val)
		case "datarow":
			if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
				opts.datarow = n
			}
		case "delimiter", "dlm":
			if val != "" {
				opts.delimiter = val
			}
		}
	}
	return opts
}

// sniffKinds infers each column's type from the data records: numeric if every
// non-empty value in the column parses as a float, otherwise character. An
// all-empty column defaults to character.
func sniffKinds(records [][]string, ncol int) []table.Kind {
	kinds := make([]table.Kind, ncol)
	seen := make([]bool, ncol)
	for i := range kinds {
		kinds[i] = table.Numeric
	}
	for _, rec := range records {
		for i := 0; i < ncol; i++ {
			if i >= len(rec) {
				continue
			}
			field := strings.TrimSpace(rec[i])
			if field == "" {
				continue
			}
			seen[i] = true
			if _, err := strconv.ParseFloat(field, 64); err != nil {
				kinds[i] = table.Character
			}
		}
	}
	for i := range kinds {
		if !seen[i] {
			kinds[i] = table.Character
		}
	}
	return kinds
}

// parseImportNum converts a field to numeric, yielding numeric missing for an
// empty/"." field.
func parseImportNum(s string) table.Value {
	s = strings.TrimSpace(s)
	if s == "" || s == "." {
		return table.MissingNum()
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return table.MissingNum()
	}
	return table.Num(f)
}

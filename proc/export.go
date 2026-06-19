package proc

import (
	"strconv"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/flatfile"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() { Register("export", exportProc{}) }

// exportProc implements PROC EXPORT for delimited flat files (DBMS=CSV/TAB/DLM):
//
//	proc export data=ds outfile="path" dbms=csv replace;
//	  putnames=yes;   /* write a header row of column names (default) */
//	  delimiter=',';  /* or dlm=, for DBMS=DLM */
//	run;
//
// Each value is rendered with DSD/CSV quoting (values containing the delimiter, a
// quote, or a newline are quoted). A missing value writes as an empty field —
// the PROC EXPORT convention, distinct from the DATA step PUT's `.` for a missing
// numeric.
type exportProc struct{}

func (exportProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	src, ok := lib.Get(step.Data)
	if !ok {
		logger.Error("PROC EXPORT: data set %s not found.", strings.ToUpper(step.Data))
		return nil
	}

	var outfile, dbms string
	for _, o := range step.Options {
		switch strings.ToLower(o.Name) {
		case "outfile", "file":
			outfile = o.Value
		case "dbms":
			dbms = strings.ToLower(o.Value)
		case "replace":
			// always replace; the output file is overwritten
		}
	}
	if outfile == "" {
		logger.Error("PROC EXPORT: OUTFILE= is required.")
		return nil
	}

	putnames := true
	var delim string
	for _, s := range step.Body {
		raw, ok := s.(*ast.RawStatement)
		if !ok {
			continue
		}
		key, val := splitRawOption(raw.Text)
		switch key {
		case "putnames":
			putnames = !isNo(val)
		case "delimiter", "dlm":
			if val != "" {
				delim = val
			}
		}
	}
	sep := delimiterFor(dbms, delim)
	sepStr := string(sep)

	var lines []string
	if putnames {
		names := src.ColumnNames()
		quoted := make([]string, len(names))
		for i, n := range names {
			quoted[i] = flatfile.Quote(n, sepStr)
		}
		lines = append(lines, strings.Join(quoted, sepStr))
	}
	for _, r := range src.Rows {
		fields := make([]string, len(src.Columns))
		for i, c := range src.Columns {
			fields[i] = flatfile.Quote(exportCell(src.Get(r, c.Name)), sepStr)
		}
		lines = append(lines, strings.Join(fields, sepStr))
	}

	if err := flatfile.WriteLines(outfile, lines); err != nil {
		logger.Error("PROC EXPORT: %v", err)
		return nil
	}
	logger.Note("%d records were written to the file %s.", src.NObs(), outfile)
	return nil
}

// exportCell renders one value for EXPORT: a missing value (numeric or
// character) is an empty field; a numeric uses the shortest round-trip
// representation; a character is its string.
func exportCell(v table.Value) string {
	if v.IsMissing() {
		return ""
	}
	if v.Kind == table.Character {
		return v.Str
	}
	return strconv.FormatFloat(v.Num, 'g', -1, 64)
}

// delimiterFor resolves the field separator from DBMS= and an explicit
// DELIMITER=/DLM= value. DBMS=TAB implies a tab; an explicit delimiter wins;
// otherwise (CSV or unspecified) the comma is used.
func delimiterFor(dbms, delimiter string) rune {
	if delimiter != "" {
		return []rune(delimiter)[0]
	}
	switch dbms {
	case "tab":
		return '\t'
	default:
		return ','
	}
}

// splitRawOption splits a `key = value` raw statement into a lowercased key and
// its (untrimmed-of-internal, trimmed-of-edges) value. A statement with no `=`
// yields the whole text as the key and an empty value.
func splitRawOption(text string) (key, value string) {
	if i := strings.Index(text, "="); i >= 0 {
		return strings.ToLower(strings.TrimSpace(text[:i])), strings.TrimSpace(text[i+1:])
	}
	return strings.ToLower(strings.TrimSpace(text)), ""
}

// isNo reports whether a yes/no option value is "no" (case-insensitive).
func isNo(val string) bool {
	return strings.EqualFold(strings.TrimSpace(val), "no")
}

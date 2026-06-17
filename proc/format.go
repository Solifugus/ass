package proc

import (
	"strconv"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() { Register("format", formatProc{}) }

// formatProc implements PROC FORMAT: it registers user-defined VALUE formats in
// the library's format catalog so later steps (e.g. PROC PRINT) can render
// values through them. Only the VALUE statement is supported (PICTURE, INVALUE,
// and format catalogs on disk are not).
type formatProc struct{}

func (formatProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	if lib.Formats == nil {
		lib.Formats = table.NewFormatCatalog()
	}
	for _, s := range step.Body {
		vs, ok := s.(*ast.ValueStatement)
		if !ok {
			continue
		}
		vf := &table.ValueFormat{Name: vs.Name, Char: vs.Char}
		for _, r := range vs.Ranges {
			vf.Ranges = append(vf.Ranges, convertRange(r, vs.Char))
		}
		lib.Formats.Define(vf)
		logger.Note("Format %s has been output.", strings.ToUpper(strings.TrimPrefix(vs.Name, "$")))
	}
	return nil
}

// convertRange translates a parsed AST range into a table.FormatRange, parsing
// numeric bounds when the format is numeric.
func convertRange(r ast.ValueRange, char bool) table.FormatRange {
	out := table.FormatRange{
		NoLow:    r.NoLow,
		NoHigh:   r.NoHigh,
		LowExcl:  r.LowExcl,
		HighExcl: r.HighExcl,
		Other:    r.Other,
		Label:    r.Label,
	}
	if r.Other {
		return out
	}
	out.Low = boundValue(r.Low, char)
	out.High = boundValue(r.High, char)
	return out
}

// boundValue converts a range-bound's source text to a table.Value.
func boundValue(s string, char bool) table.Value {
	if char {
		return table.Char(s)
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return table.MissingNum()
	}
	return table.Num(n)
}

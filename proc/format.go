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
// values through them. VALUE, INVALUE (user informats), and PICTURE (output
// templates) statements are supported; format catalogs on disk are not.
type formatProc struct{}

func (formatProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	if lib.Formats == nil {
		lib.Formats = table.NewFormatCatalog()
	}
	if lib.Informats == nil {
		lib.Informats = table.NewInformatCatalog()
	}

	var cntlin, cntlout string
	for _, o := range step.Options {
		switch strings.ToLower(o.Name) {
		case "cntlin":
			cntlin = o.Value
		case "cntlout":
			cntlout = o.Value
		}
	}
	// CNTLIN= builds formats from a control dataset before this step's own VALUE
	// statements (which may then add to or override them).
	if cntlin != "" {
		if err := formatCntlin(lib, cntlin, logger); err != nil {
			logger.Error("PROC FORMAT CNTLIN=: %v", err)
		}
	}

	for _, s := range step.Body {
		vs, ok := s.(*ast.ValueStatement)
		if !ok {
			continue
		}
		if vs.Invalue {
			inf := &table.UserInformat{Name: vs.Name, Char: vs.Char}
			for _, r := range vs.Ranges {
				inf.Ranges = append(inf.Ranges, convertInformatRange(r, vs.Char))
			}
			lib.Informats.Define(inf)
			logger.Note("Informat %s has been output.", strings.ToUpper(strings.TrimPrefix(vs.Name, "$")))
			continue
		}
		vf := &table.ValueFormat{Name: vs.Name, Char: vs.Char, Picture: vs.Picture}
		for _, r := range vs.Ranges {
			cr := convertRange(r, vs.Char)
			if vs.Picture {
				applyPictureOptions(&cr, r)
			}
			vf.Ranges = append(vf.Ranges, cr)
		}
		lib.Formats.Define(vf)
		logger.Note("Format %s has been output.", strings.ToUpper(strings.TrimPrefix(vs.Name, "$")))
	}

	// CNTLOUT= writes the catalog's VALUE formats to a control dataset.
	if cntlout != "" {
		if err := formatCntlout(lib, cntlout, logger); err != nil {
			logger.Error("PROC FORMAT CNTLOUT=: %v", err)
		}
	}
	return nil
}

// convertInformatRange translates a parsed VALUE-style range into an
// InformatRange. A range whose endpoints parse as numbers is a numeric interval
// matched against the input value; otherwise it is a string key matched exactly.
// The label is the result: a number for a numeric informat, a string for a `$`
// (character) informat.
func convertInformatRange(r ast.ValueRange, char bool) table.InformatRange {
	out := table.InformatRange{Other: r.Other}
	if char {
		out.Result = table.Char(r.Label)
	} else if n, err := strconv.ParseFloat(r.Label, 64); err == nil {
		out.Result = table.Num(n)
	} else {
		out.Result = table.MissingNum()
	}
	if r.Other {
		return out
	}
	lo, loNum := parseFloatOK(r.Low)
	hi, hiNum := parseFloatOK(r.High)
	// Numeric interval only when both present bounds parse as numbers.
	if (r.NoLow || loNum) && (r.NoHigh || hiNum) && (loNum || hiNum) {
		out.Numeric = true
		out.Low, out.High = lo, hi
		out.NoLow, out.NoHigh = r.NoLow, r.NoHigh
		out.LowExcl, out.HighExcl = r.LowExcl, r.HighExcl
		return out
	}
	out.Key = r.Low // exact string-key match
	return out
}

// parseFloatOK reports whether s parses as a float (and its value).
func parseFloatOK(s string) (float64, bool) {
	n, err := strconv.ParseFloat(s, 64)
	return n, err == nil
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

// applyPictureOptions copies a PICTURE range's parenthesized options (prefix,
// mult, fill) from the parsed AST range onto the table.FormatRange.
func applyPictureOptions(cr *table.FormatRange, r ast.ValueRange) {
	cr.Prefix = r.Prefix
	if r.Mult != "" {
		if m, err := strconv.ParseFloat(r.Mult, 64); err == nil {
			cr.Mult = m
		}
	}
	if r.Fill != "" {
		cr.Fill = r.Fill[0]
	}
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

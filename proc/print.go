package proc

import (
	"fmt"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() { Register("print", printProc{}) }

// printProc implements PROC PRINT: it renders a dataset as a SAS-style listing.
type printProc struct{}

// printOptions captures the options that affect the listing.
type printOptions struct {
	noobs bool     // suppress the Obs column
	vars  []string // explicit column selection/order (empty = all columns)
}

func (printProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	ds, ok := lib.Get(step.Data)
	if !ok {
		logger.Error("PROC PRINT: data set %s not found.", strings.ToUpper(step.Data))
		return nil
	}
	opts := parsePrintOptions(step)
	fmt.Print(renderListing(ds, opts))
	logger.Note("There were %d observations read from the data set %s.%s.",
		ds.NObs(), strings.ToUpper(ds.Lib), strings.ToUpper(ds.Name))
	return nil
}

// parsePrintOptions reads PROC PRINT options (step header) and statements (VAR).
func parsePrintOptions(step *ast.ProcStep) printOptions {
	var opts printOptions
	for _, o := range step.Options {
		if strings.EqualFold(o.Name, "noobs") {
			opts.noobs = true
		}
	}
	for _, s := range step.Body {
		if v, ok := s.(*ast.VarStatement); ok {
			opts.vars = append(opts.vars, v.Vars...)
		}
	}
	return opts
}

// listingColumn is one rendered column: its header, display width, and whether
// its values are right-aligned (numeric).
type listingColumn struct {
	name  string
	width int
	right bool
}

// renderListing produces the SAS-style listing text for a dataset. The format
// (locked as the ASS listing format): an "Obs" column (unless noobs) holding the
// 1-based row number, then the selected variables; numeric columns are
// right-aligned and character columns left-aligned; headers align with their
// data; columns are separated by a two-space gutter; a blank line separates the
// header from the data rows.
func renderListing(ds *table.Dataset, opts printOptions) string {
	cols := selectColumns(ds, opts.vars)

	// Compute column widths from headers and cell values.
	lc := make([]listingColumn, len(cols))
	for i, c := range cols {
		width := len(c.Name)
		right := c.Kind == table.Numeric
		for _, r := range ds.Rows {
			if w := len(ds.Get(r, c.Name).Display()); w > width {
				width = w
			}
		}
		lc[i] = listingColumn{name: c.Name, width: width, right: right}
	}

	obsWidth := len("Obs")
	if w := len(fmt.Sprintf("%d", ds.NObs())); w > obsWidth {
		obsWidth = w
	}

	var b strings.Builder

	// Header row.
	var head []string
	if !opts.noobs {
		head = append(head, pad("Obs", obsWidth, true))
	}
	for _, c := range lc {
		head = append(head, pad(c.name, c.width, c.right))
	}
	b.WriteString(strings.TrimRight(strings.Join(head, "  "), " "))
	b.WriteString("\n\n")

	// Data rows.
	for i, r := range ds.Rows {
		var cells []string
		if !opts.noobs {
			cells = append(cells, pad(fmt.Sprintf("%d", i+1), obsWidth, true))
		}
		for _, c := range lc {
			cells = append(cells, pad(ds.Get(r, c.name).Display(), c.width, c.right))
		}
		b.WriteString(strings.TrimRight(strings.Join(cells, "  "), " "))
		b.WriteString("\n")
	}
	return b.String()
}

// selectColumns resolves the columns to print: the VAR list (existing columns,
// in the given order) if provided, otherwise all columns in dataset order.
func selectColumns(ds *table.Dataset, vars []string) []table.Column {
	if len(vars) == 0 {
		return ds.Columns
	}
	var out []table.Column
	for _, name := range vars {
		for _, c := range ds.Columns {
			if strings.EqualFold(c.Name, name) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// pad left- or right-justifies s within width.
func pad(s string, width int, right bool) string {
	if len(s) >= width {
		return s
	}
	gap := strings.Repeat(" ", width-len(s))
	if right {
		return gap + s
	}
	return s + gap
}

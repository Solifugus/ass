package proc

import (
	"fmt"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/formats"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() { Register("print", printProc{}) }

// printProc implements PROC PRINT: it renders a dataset as a SAS-style listing.
type printProc struct{}

// printOptions captures the options that affect the listing.
type printOptions struct {
	noobs   bool              // suppress the Obs column
	label   bool              // use column labels (when set) as headers
	vars    []string          // explicit column selection/order (empty = all columns)
	formats map[string]string // var (lowercased) -> format override (from a FORMAT statement)
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
		switch strings.ToLower(o.Name) {
		case "noobs":
			opts.noobs = true
		case "label":
			opts.label = true
		}
	}
	for _, s := range step.Body {
		switch st := s.(type) {
		case *ast.VarStatement:
			opts.vars = append(opts.vars, st.Vars...)
		case *ast.FormatStatement:
			if opts.formats == nil {
				opts.formats = map[string]string{}
			}
			for k, v := range st.Formats {
				opts.formats[k] = v
			}
		}
	}
	return opts
}

// listingColumn is one rendered column: its variable name, the header text to
// display, its width, and whether its values are right-aligned (numeric).
type listingColumn struct {
	name   string
	header string
	width  int
	right  bool
	format string
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
	colFormat := func(c table.Column) string {
		if f, ok := opts.formats[strings.ToLower(c.Name)]; ok {
			return f
		}
		return c.Format
	}

	lc := make([]listingColumn, len(cols))
	for i, c := range cols {
		header := c.Name
		if opts.label && c.Label != "" {
			header = c.Label
		}
		width := len(header)
		right := c.Kind == table.Numeric
		for _, r := range ds.Rows {
			if w := len(formats.Apply(ds.Get(r, c.Name), colFormat(c))); w > width {
				width = w
			}
		}
		lc[i] = listingColumn{name: c.Name, header: header, width: width, right: right, format: colFormat(c)}
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
		head = append(head, pad(c.header, c.width, c.right))
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
			cells = append(cells, pad(formats.Apply(ds.Get(r, c.name), c.format), c.width, c.right))
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

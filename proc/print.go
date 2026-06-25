package proc

import (
	"fmt"
	"html"
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
	noobs   bool                 // suppress the Obs column
	label   bool                 // use column labels (when set) as headers
	vars    []string             // explicit column selection/order (empty = all columns)
	formats map[string]string    // var (lowercased) -> format override (from a FORMAT statement)
	labels  map[string]string    // var (lowercased) -> label override (from a LABEL statement)
	catalog *table.FormatCatalog // user-defined formats (from PROC FORMAT), may be nil
}

func (printProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	ds, ok := lib.Get(step.Data)
	if !ok {
		logger.Error("PROC PRINT: data set %s not found.", strings.ToUpper(step.Data))
		return nil
	}
	opts := parsePrintOptions(step)
	opts.catalog = lib.Formats
	emitListing(logger, ds, opts)
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
		case *ast.LabelStatement:
			if opts.labels == nil {
				opts.labels = map[string]string{}
			}
			for k, v := range st.Labels {
				opts.labels[k] = v
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
		if opts.label {
			// A LABEL statement in this step overrides the variable's stored label.
			if lbl, ok := opts.labels[strings.ToLower(c.Name)]; ok && lbl != "" {
				header = lbl
			} else if c.Label != "" {
				header = c.Label
			}
		}
		width := len(header)
		right := c.Kind == table.Numeric
		for _, r := range ds.Rows {
			if w := len(applyFmt(opts.catalog, ds.Get(r, c.Name), colFormat(c))); w > width {
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
			cells = append(cells, pad(applyFmt(opts.catalog, ds.Get(r, c.name), c.format), c.width, c.right))
		}
		b.WriteString(strings.TrimRight(strings.Join(cells, "  "), " "))
		b.WriteString("\n")
	}
	return b.String()
}

// emitListing outputs a dataset's PROC listing: always the plain-text table, and
// — only when the logger has a rich sink (the Jupyter kernel) — an HTML table
// too. Outside a rich frontend this is identical to writing renderListing to the
// listing stream, so batch and REPL output is unchanged.
func emitListing(logger *log.Logger, ds *table.Dataset, opts printOptions) {
	text := renderListing(ds, opts)
	htmlOut := ""
	if logger.Rich() {
		htmlOut = renderHTMLListing(ds, opts)
	}
	logger.EmitTable(text, htmlOut)
}

// renderHTMLListing renders the same listing as renderListing but as an HTML
// table, for rich frontends. It reuses the identical column selection, header/
// label resolution, and value formatting, so the cells match the text form.
func renderHTMLListing(ds *table.Dataset, opts printOptions) string {
	cols := selectColumns(ds, opts.vars)
	colFormat := func(c table.Column) string {
		if f, ok := opts.formats[strings.ToLower(c.Name)]; ok {
			return f
		}
		return c.Format
	}

	var b strings.Builder
	b.WriteString(`<table class="ass-table" style="border-collapse:collapse" border="1" cellpadding="4">`)
	b.WriteString("<thead><tr>")
	if !opts.noobs {
		b.WriteString("<th>Obs</th>")
	}
	for _, c := range cols {
		header := c.Name
		if opts.label {
			if lbl, ok := opts.labels[strings.ToLower(c.Name)]; ok && lbl != "" {
				header = lbl
			} else if c.Label != "" {
				header = c.Label
			}
		}
		b.WriteString("<th>" + html.EscapeString(header) + "</th>")
	}
	b.WriteString("</tr></thead><tbody>")

	for i, r := range ds.Rows {
		b.WriteString("<tr>")
		if !opts.noobs {
			fmt.Fprintf(&b, "<td>%d</td>", i+1)
		}
		for _, c := range cols {
			cell := html.EscapeString(applyFmt(opts.catalog, ds.Get(r, c.Name), colFormat(c)))
			if c.Kind == table.Numeric {
				b.WriteString(`<td style="text-align:right">` + cell + "</td>")
			} else {
				b.WriteString("<td>" + cell + "</td>")
			}
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

// applyFmt renders v through a format spec, consulting the user-format catalog
// first (PROC FORMAT VALUE formats) and falling back to the built-in formats.
// When a named user format exists but v matches none of its ranges, v is shown
// with its default display, matching SAS.
func applyFmt(cat *table.FormatCatalog, v table.Value, spec string) string {
	if cat != nil && spec != "" {
		key := strings.TrimSuffix(strings.ToLower(spec), ".")
		if vf, ok := lookupUserFormat(cat, key); ok {
			if label, matched := vf.Format(v); matched {
				return label
			}
			return v.Display()
		}
	}
	return formats.Apply(v, spec)
}

// lookupUserFormat finds a user format by name, retrying with any trailing
// display-width digits stripped (e.g. "agegrp8" -> "agegrp").
func lookupUserFormat(cat *table.FormatCatalog, key string) (*table.ValueFormat, bool) {
	if vf, ok := cat.Lookup(key); ok {
		return vf, true
	}
	bare := strings.TrimRight(key, "0123456789")
	if bare != key && bare != "" && bare != "$" {
		return cat.Lookup(bare)
	}
	return nil, false
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

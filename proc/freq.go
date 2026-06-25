package proc

import (
	"fmt"
	"html"
	"math"
	"sort"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() { Register("freq", freqProc{}) }

// freqProc implements PROC FREQ: one-way frequency tables (Frequency, Percent,
// cumulative Frequency, cumulative Percent) for each TABLES variable.
type freqProc struct{}

func (freqProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	src, ok := lib.Get(step.Data)
	if !ok {
		logger.Error("PROC FREQ: data set %s not found.", strings.ToUpper(step.Data))
		return nil
	}

	type freqReq struct {
		vars []string
		opts *ast.TablesStatement
	}
	var requests []freqReq
	procFormats := map[string]string{}
	for _, s := range step.Body {
		switch st := s.(type) {
		case *ast.TablesStatement:
			for _, req := range st.Requests {
				requests = append(requests, freqReq{vars: req, opts: st})
			}
		case *ast.VarStatement:
			for _, v := range st.Vars {
				requests = append(requests, freqReq{vars: []string{v}})
			}
		case *ast.FormatStatement:
			for k, v := range st.Formats {
				procFormats[strings.ToLower(k)] = v
			}
		}
	}
	if len(requests) == 0 {
		logger.Error("PROC FREQ: a TABLES statement is required.")
		return nil
	}

	fmtFor := func(v string) func(table.Value) string {
		return freqFormatter(src, lib.Formats, procFormats, v)
	}
	fmtdFor := func(v string) bool { return varFormatSpec(src, procFormats, v) != "" }
	has := func(o *ast.TablesStatement, opt string) bool { return o != nil && o.HasOption(opt) }

	emitTitles(logger, lib.TitleLines())
	defer emitFootnotes(logger, lib.FootnoteLines())
	for _, req := range requests {
		switch {
		case len(req.vars) == 0:
			continue
		case len(req.vars) == 1 || has(req.opts, "list"):
			// One-way, or an n-way list-format table (all distinct combinations).
			res := buildFreqResultN(src, req.vars, fmtFor, fmtdFor)
			res = applyFreqOptions(res, req.opts)
			emitListing(logger, res, printOptions{}, "Frequencies")
			fmt.Fprintln(logger.Listing())
		default:
			// Two (or more) variables: cross-tabulate the first two. The
			// `/ nofreq nopercent norow nocol` options suppress the matching cell
			// statistic (frequency / cell percent / row percent / column percent).
			rf, cf := fmtFor(req.vars[0]), fmtFor(req.vars[1])
			showFreq := !has(req.opts, "nofreq")
			showPct := !has(req.opts, "nopercent")
			showRow := !has(req.opts, "norow")
			showCol := !has(req.opts, "nocol")
			ctHTML := ""
			if logger.Rich() {
				ctHTML = renderCrossTabHTML(src, req.vars[0], req.vars[1], rf, cf, showFreq, showPct, showRow, showCol)
			}
			logger.EmitTable(renderCrossTab(src, req.vars[0], req.vars[1], rf, cf, showFreq, showPct, showRow, showCol), ctHTML)
			if has(req.opts, "chisq") {
				csHTML := ""
				if logger.Rich() {
					csHTML = renderChiSquareHTML(src, req.vars[0], req.vars[1], rf, cf)
				}
				logger.EmitTable(renderChiSquare(src, req.vars[0], req.vars[1], rf, cf), csHTML)
			}
			fmt.Fprintln(logger.Listing())
		}
	}
	return nil
}

// applyFreqOptions drops result columns suppressed by `/ options`: nofreq removes
// Frequency, nopercent removes Percent/CumPercent, nocum removes the cumulative
// columns. The category and any remaining columns are preserved in order.
func applyFreqOptions(res *table.Dataset, opts *ast.TablesStatement) *table.Dataset {
	if opts == nil {
		return res
	}
	drop := map[string]bool{}
	if opts.HasOption("nofreq") {
		drop["frequency"] = true
	}
	if opts.HasOption("nopercent") {
		drop["percent"], drop["cumpercent"] = true, true
	}
	if opts.HasOption("nocum") {
		drop["cumfreq"], drop["cumpercent"] = true, true
	}
	if len(drop) == 0 {
		return res
	}
	out := table.NewDataset(res.Lib, res.Name)
	var keep []string
	for _, c := range res.Columns {
		if drop[strings.ToLower(c.Name)] {
			continue
		}
		out.AddColumn(c)
		keep = append(keep, strings.ToLower(c.Name))
	}
	for _, r := range res.Rows {
		nr := table.Row{}
		for _, k := range keep {
			nr[k] = r[k]
		}
		out.AppendRow(nr)
	}
	return out
}

// varFormatSpec returns the effective format spec for variable v: a FORMAT
// statement in the PROC takes precedence over the column's stored format.
func varFormatSpec(src *table.Dataset, procFormats map[string]string, v string) string {
	if f, ok := procFormats[strings.ToLower(v)]; ok {
		return f
	}
	for _, c := range src.Columns {
		if strings.EqualFold(c.Name, v) {
			return c.Format
		}
	}
	return ""
}

// freqFormatter returns a function that maps a value of variable v to the label
// FREQ groups and displays it under: the variable's user/built-in format if one
// applies, else Value.Display(). User VALUE formats (from PROC FORMAT) let FREQ
// collapse several underlying values into one formatted category, matching SAS.
func freqFormatter(src *table.Dataset, cat *table.FormatCatalog, procFormats map[string]string, v string) func(table.Value) string {
	spec := varFormatSpec(src, procFormats, v)
	return func(val table.Value) string {
		if spec == "" {
			return val.Display()
		}
		return applyFmt(cat, val, spec)
	}
}

// sortedDistinct returns the non-missing distinct formatted categories of
// variable v in src, in SAS sort order. Grouping is by the formatted label
// (fmtFn), so a user format collapses underlying values into one category; the
// returned label/min-value pairs order by the smallest underlying value.
func sortedDistinct(src *table.Dataset, v string, fmtFn func(table.Value) string) []freqCat {
	idx := map[string]int{}
	var cats []freqCat
	for _, r := range src.Rows {
		val := src.Get(r, v)
		if val.IsMissing() {
			continue
		}
		k := fmtFn(val)
		if i, ok := idx[k]; ok {
			if val.Compare(cats[i].min) < 0 {
				cats[i].min = val
			}
			continue
		}
		idx[k] = len(cats)
		cats = append(cats, freqCat{label: k, min: val})
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i].min.Compare(cats[j].min) < 0 })
	return cats
}

// freqCat is a distinct FREQ category: the formatted label and the smallest
// underlying value mapped to it (used for ordering).
type freqCat struct {
	label string
	min   table.Value
}

// renderCrossTab renders a SAS-style two-way frequency cross-tabulation of
// rowVar by colVar. Each interior cell stacks Frequency, Percent (of grand
// total), Row Pct, and Col Pct; the right and bottom margins carry row/column
// totals (Frequency and Percent). Missing values in either variable are
// excluded.
func renderCrossTab(src *table.Dataset, rowVar, colVar string, rowFmt, colFmt func(table.Value) string, showFreq, showPct, showRow, showCol bool) string {
	rowVals := sortedDistinct(src, rowVar, rowFmt)
	colVals := sortedDistinct(src, colVar, colFmt)

	count := map[string]map[string]int{}
	rowTot := map[string]int{}
	colTot := map[string]int{}
	grand := 0
	for _, r := range src.Rows {
		rv, cv := src.Get(r, rowVar), src.Get(r, colVar)
		if rv.IsMissing() || cv.IsMissing() {
			continue
		}
		rk, ck := rowFmt(rv), colFmt(cv)
		if count[rk] == nil {
			count[rk] = map[string]int{}
		}
		count[rk][ck]++
		rowTot[rk]++
		colTot[ck]++
		grand++
	}

	pct := func(n, d int) string {
		if d == 0 {
			return "."
		}
		return fmt.Sprintf("%.2f", 100*float64(n)/float64(d))
	}

	// Active interior-cell statistics in SAS stacking order, after applying the
	// `/ nofreq nopercent norow nocol` suppression options. At least one is always
	// shown (if all four are suppressed, fall back to Frequency). cell() gives the
	// interior value for (rk,ck); rmar() the right-margin (row-total) value, "" if
	// that statistic has no row-total margin.
	type ctStat struct {
		label string
		cell  func(rk, ck string) string
		rmar  func(rk string) string
	}
	var stats []ctStat
	if showFreq {
		stats = append(stats, ctStat{"Frequency",
			func(rk, ck string) string { return fmt.Sprintf("%d", count[rk][ck]) },
			func(rk string) string { return fmt.Sprintf("%d", rowTot[rk]) }})
	}
	if showPct {
		stats = append(stats, ctStat{"Percent",
			func(rk, ck string) string { return pct(count[rk][ck], grand) },
			func(rk string) string { return pct(rowTot[rk], grand) }})
	}
	if showRow {
		stats = append(stats, ctStat{"Row Pct",
			func(rk, ck string) string { return pct(count[rk][ck], rowTot[rk]) },
			func(string) string { return "" }})
	}
	if showCol {
		stats = append(stats, ctStat{"Col Pct",
			func(rk, ck string) string { return pct(count[rk][ck], colTot[ck]) },
			func(string) string { return "" }})
	}
	if len(stats) == 0 {
		stats = append(stats, ctStat{"Frequency",
			func(rk, ck string) string { return fmt.Sprintf("%d", count[rk][ck]) },
			func(rk string) string { return fmt.Sprintf("%d", rowTot[rk]) }})
	}

	// Column headers: each colVal, then "Total". The left stub holds the legend
	// labels and the row-variable values.
	headers := make([]string, 0, len(colVals)+1)
	for _, cv := range colVals {
		headers = append(headers, cv.label)
	}
	headers = append(headers, "Total")

	stubW := len(rowVar)
	for _, s := range stats {
		if len(s.label) > stubW {
			stubW = len(s.label)
		}
	}
	for _, rv := range rowVals {
		if w := len(rv.label); w > stubW {
			stubW = w
		}
	}
	if len("Total") > stubW {
		stubW = len("Total")
	}

	// Pre-compute every data column's content width from the active stats only.
	colW := make([]int, len(headers))
	for i, h := range headers {
		colW[i] = len(h)
	}
	widen := func(i int, s string) {
		if len(s) > colW[i] {
			colW[i] = len(s)
		}
	}
	last := len(headers) - 1
	for _, rv := range rowVals {
		rk := rv.label
		for j, cv := range colVals {
			for _, s := range stats {
				widen(j, s.cell(rk, cv.label))
			}
		}
		for _, s := range stats {
			widen(last, s.rmar(rk))
		}
	}
	for j, cv := range colVals {
		ck := cv.label
		if showFreq {
			widen(j, fmt.Sprintf("%d", colTot[ck]))
		}
		if showPct {
			widen(j, pct(colTot[ck], grand))
		}
	}
	if showFreq {
		widen(last, fmt.Sprintf("%d", grand))
	}
	if showPct {
		widen(last, pct(grand, grand))
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Table of %s by %s\n\n", rowVar, colVar))

	line := func(stub string, cells []string) {
		parts := pad(stub, stubW, false)
		for i, c := range cells {
			parts += "  " + pad(c, colW[i], true)
		}
		b.WriteString(strings.TrimRight(parts, " ") + "\n")
	}

	// Header band: the column-variable name, the row-variable name, then the active
	// stat legend labels — all but the last on their own line, the last sharing the
	// line that carries the column value headers.
	b.WriteString(strings.Repeat(" ", stubW) + "  " + colVar + "\n")
	b.WriteString(rowVar + "\n")
	for i := 0; i < len(stats)-1; i++ {
		b.WriteString(stats[i].label + "\n")
	}
	line(stats[len(stats)-1].label, headers)
	b.WriteString("\n")

	// Body: one band of (#active stats) lines per row value, with the row value
	// labeling the first line and the rest left blank.
	for _, rv := range rowVals {
		rk := rv.label
		for si, s := range stats {
			cells := make([]string, 0, len(colVals)+1)
			for _, cv := range colVals {
				cells = append(cells, s.cell(rk, cv.label))
			}
			cells = append(cells, s.rmar(rk))
			stub := ""
			if si == 0 {
				stub = rk
			}
			line(stub, cells)
		}
		b.WriteString("\n")
	}

	// Bottom margin: column totals (Frequency and/or Percent only).
	firstBottom := true
	if showFreq {
		ct := make([]string, 0, len(colVals)+1)
		for _, cv := range colVals {
			ct = append(ct, fmt.Sprintf("%d", colTot[cv.label]))
		}
		ct = append(ct, fmt.Sprintf("%d", grand))
		line("Total", ct)
		firstBottom = false
	}
	if showPct {
		cpc := make([]string, 0, len(colVals)+1)
		for _, cv := range colVals {
			cpc = append(cpc, pct(colTot[cv.label], grand))
		}
		cpc = append(cpc, pct(grand, grand))
		lbl := "Percent"
		if firstBottom {
			lbl = "Total"
		}
		line(lbl, cpc)
	}

	return b.String()
}

// renderCrossTabHTML renders the same contingency table as renderCrossTab but as
// a styled HTML table for rich frontends. Each body cell stacks the four SAS
// stats — frequency (bold), then cell/row/column percent (dimmed) — and the
// margins carry the row, column, and grand totals.
func renderCrossTabHTML(src *table.Dataset, rowVar, colVar string, rowFmt, colFmt func(table.Value) string, showFreq, showPct, showRow, showCol bool) string {
	rowVals := sortedDistinct(src, rowVar, rowFmt)
	colVals := sortedDistinct(src, colVar, colFmt)

	count := map[string]map[string]int{}
	rowTot := map[string]int{}
	colTot := map[string]int{}
	grand := 0
	for _, r := range src.Rows {
		rv, cv := src.Get(r, rowVar), src.Get(r, colVar)
		if rv.IsMissing() || cv.IsMissing() {
			continue
		}
		rk, ck := rowFmt(rv), colFmt(cv)
		if count[rk] == nil {
			count[rk] = map[string]int{}
		}
		count[rk][ck]++
		rowTot[rk]++
		colTot[ck]++
		grand++
	}
	if !showFreq && !showPct && !showRow && !showCol {
		showFreq = true
	}
	pct := func(n, d int) string {
		if d == 0 {
			return "."
		}
		return fmt.Sprintf("%.2f", 100*float64(n)/float64(d))
	}
	dim := `<div style="opacity:.6;font-size:11px">`
	// An interior cell stacks only the active statistics.
	cell := func(n, rowT, colT int) string {
		var s strings.Builder
		if showFreq {
			fmt.Fprintf(&s, `<div style="font-weight:600">%d</div>`, n)
		}
		if showPct {
			s.WriteString(dim + pct(n, grand) + `</div>`)
		}
		if showRow {
			s.WriteString(dim + pct(n, rowT) + `</div>`)
		}
		if showCol {
			s.WriteString(dim + pct(n, colT) + `</div>`)
		}
		return s.String()
	}
	// A margin cell carries the total's frequency and/or grand-percent only.
	marginFreqPct := func(n int) string {
		var s strings.Builder
		if showFreq {
			fmt.Fprintf(&s, `<div style="font-weight:600">%d</div>`, n)
		}
		if showPct {
			s.WriteString(dim + pct(n, grand) + `%</div>`)
		}
		return s.String()
	}

	legendParts := []string{}
	if showFreq {
		legendParts = append(legendParts, "freq")
	}
	if showPct {
		legendParts = append(legendParts, "cell %")
	}
	if showRow {
		legendParts = append(legendParts, "row %")
	}
	if showCol {
		legendParts = append(legendParts, "col %")
	}

	var b strings.Builder
	b.WriteString(`<table style="` + htmlTableStyle + `">`)
	fmt.Fprintf(&b, `<caption style="%s">Table of %s by %s<span style="font-weight:400;opacity:.6;margin-left:.55em">%s</span></caption>`,
		htmlCaptionStyle, html.EscapeString(rowVar), html.EscapeString(colVar), html.EscapeString(strings.Join(legendParts, " / ")))

	b.WriteString(`<thead><tr><th style="` + htmlThStyle + htmlTextStyle + `">` + html.EscapeString(rowVar) + " \\ " + html.EscapeString(colVar) + `</th>`)
	for _, cv := range colVals {
		b.WriteString(`<th style="` + htmlThStyle + htmlNumStyle + `">` + html.EscapeString(cv.label) + `</th>`)
	}
	b.WriteString(`<th style="` + htmlThStyle + htmlNumStyle + `">Total</th></tr></thead><tbody>`)

	for i, rv := range rowVals {
		rk := rv.label
		if i%2 == 1 {
			b.WriteString(`<tr style="` + htmlZebraStyle + `">`)
		} else {
			b.WriteString("<tr>")
		}
		b.WriteString(`<th scope="row" style="` + htmlTdStyle + htmlTextStyle + `;font-weight:600">` + html.EscapeString(rk) + `</th>`)
		for _, cv := range colVals {
			n := count[rk][cv.label]
			b.WriteString(`<td style="` + htmlTdStyle + htmlNumStyle + `">` + cell(n, rowTot[rk], colTot[cv.label]) + `</td>`)
		}
		b.WriteString(`<td style="` + htmlTdStyle + htmlNumStyle + `">` + marginFreqPct(rowTot[rk]) + `</td>`)
		b.WriteString("</tr>")
	}

	b.WriteString(`<tr><th scope="row" style="` + htmlThStyle + htmlTextStyle + `">Total</th>`)
	for _, cv := range colVals {
		b.WriteString(`<td style="` + htmlThStyle + htmlNumStyle + `">` + marginFreqPct(colTot[cv.label]) + `</td>`)
	}
	grandCell := ""
	if showFreq {
		grandCell = fmt.Sprintf(`<div style="font-weight:600">%d</div>`, grand)
	}
	b.WriteString(`<td style="` + htmlThStyle + htmlNumStyle + `">` + grandCell + `</td></tr></tbody></table>`)
	return b.String()
}

// buildFreqResult builds the one-way frequency table for a single variable.
// Categories are grouped by the formatted label (fmtFn); when formatted is true
// (a user/built-in format applies) the category column holds the character label
// and several underlying values may collapse into one category, otherwise it
// holds the underlying value as before.
func buildFreqResult(src *table.Dataset, v string, fmtFn func(table.Value) string, formatted bool) *table.Dataset {
	kind := table.Character
	for _, c := range src.Columns {
		if strings.EqualFold(c.Name, v) {
			kind = c.Kind
		}
	}

	counts := map[string]int{}
	minVal := map[string]table.Value{} // smallest underlying value per label (ordering)
	total := 0
	for _, r := range src.Rows {
		val := src.Get(r, v)
		if val.IsMissing() {
			continue // SAS excludes missing from one-way tables by default
		}
		key := fmtFn(val)
		if _, seen := counts[key]; !seen {
			minVal[key] = val
		} else if val.Compare(minVal[key]) < 0 {
			minVal[key] = val
		}
		counts[key]++
		total++
	}

	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return minVal[keys[i]].Compare(minVal[keys[j]]) < 0 })

	catKind := kind
	if formatted {
		catKind = table.Character
	}
	out := table.NewDataset("", "_freq_")
	out.AddColumn(table.Column{Name: v, Kind: catKind})
	out.AddColumn(table.Column{Name: "Frequency", Kind: table.Numeric})
	out.AddColumn(table.Column{Name: "Percent", Kind: table.Numeric, Format: "5.1"})
	out.AddColumn(table.Column{Name: "CumFreq", Kind: table.Numeric})
	out.AddColumn(table.Column{Name: "CumPercent", Kind: table.Numeric, Format: "5.1"})

	cum := 0
	for _, k := range keys {
		cum += counts[k]
		cat := minVal[k]
		if formatted {
			cat = table.Char(k)
		}
		row := table.Row{
			strings.ToLower(v): cat,
			"frequency":        table.Num(float64(counts[k])),
			"percent":          pctVal(counts[k], total),
			"cumfreq":          table.Num(float64(cum)),
			"cumpercent":       pctVal(cum, total),
		}
		out.AppendRow(row)
	}
	return out
}

// buildFreqResultN builds a list-format frequency table over one or more
// variables: each distinct combination of (formatted) category values is a row
// with the category columns followed by Frequency/Percent/CumFreq/CumPercent.
// For a single variable this is the one-way table; for several it is the SAS
// `tables a*b*c / list` style. Combinations are ordered by underlying value,
// position by position.
func buildFreqResultN(src *table.Dataset, vars []string, fmtFor func(string) func(table.Value) string, fmtdFor func(string) bool) *table.Dataset {
	n := len(vars)
	fmts := make([]func(table.Value) string, n)
	formatted := make([]bool, n)
	kinds := make([]table.Kind, n)
	for i, v := range vars {
		fmts[i] = fmtFor(v)
		formatted[i] = fmtdFor(v)
		kinds[i] = table.Character
		for _, c := range src.Columns {
			if strings.EqualFold(c.Name, v) {
				kinds[i] = c.Kind
			}
		}
	}

	type combo struct {
		labels []string      // per-var formatted label
		mins   []table.Value // per-var smallest underlying value (ordering)
	}
	counts := map[string]int{}
	combos := map[string]*combo{}
	var keys []string
	for _, r := range src.Rows {
		labels := make([]string, n)
		vals := make([]table.Value, n)
		missing := false
		for i, v := range vars {
			val := src.Get(r, v)
			if val.IsMissing() {
				missing = true
				break
			}
			vals[i] = val
			labels[i] = fmts[i](val)
		}
		if missing {
			continue // SAS excludes rows with any missing classification value
		}
		key := strings.Join(labels, "\x00")
		if c, ok := combos[key]; ok {
			for i := range vals {
				if vals[i].Compare(c.mins[i]) < 0 {
					c.mins[i] = vals[i]
				}
			}
		} else {
			cp := make([]table.Value, n)
			copy(cp, vals)
			combos[key] = &combo{labels: labels, mins: cp}
			keys = append(keys, key)
		}
		counts[key]++
	}

	sort.Slice(keys, func(a, b int) bool {
		ca, cb := combos[keys[a]], combos[keys[b]]
		for i := 0; i < n; i++ {
			if cmp := ca.mins[i].Compare(cb.mins[i]); cmp != 0 {
				return cmp < 0
			}
		}
		return false
	})

	total := 0
	for _, k := range keys {
		total += counts[k]
	}

	out := table.NewDataset("", "_freq_")
	for i, v := range vars {
		k := kinds[i]
		if formatted[i] {
			k = table.Character
		}
		out.AddColumn(table.Column{Name: v, Kind: k})
	}
	out.AddColumn(table.Column{Name: "Frequency", Kind: table.Numeric})
	out.AddColumn(table.Column{Name: "Percent", Kind: table.Numeric, Format: "5.1"})
	out.AddColumn(table.Column{Name: "CumFreq", Kind: table.Numeric})
	out.AddColumn(table.Column{Name: "CumPercent", Kind: table.Numeric, Format: "5.1"})

	cum := 0
	for _, k := range keys {
		c := combos[k]
		cum += counts[k]
		row := table.Row{}
		for i, v := range vars {
			if formatted[i] {
				row[strings.ToLower(v)] = table.Char(c.labels[i])
			} else {
				row[strings.ToLower(v)] = c.mins[i]
			}
		}
		row["frequency"] = table.Num(float64(counts[k]))
		row["percent"] = pctVal(counts[k], total)
		row["cumfreq"] = table.Num(float64(cum))
		row["cumpercent"] = pctVal(cum, total)
		out.AppendRow(row)
	}
	return out
}

// chiSquareStat computes the Pearson chi-square statistic, its degrees of
// freedom, and the right-tail p-value for the two-way table of rowVar by colVar
// (grouped by the formatted categories). Rows with a missing value in either
// variable are excluded, matching the cross-tabulation.
func chiSquareStat(src *table.Dataset, rowVar, colVar string, rowFmt, colFmt func(table.Value) string) (stat float64, df int, p float64) {
	rowCats := sortedDistinct(src, rowVar, rowFmt)
	colCats := sortedDistinct(src, colVar, colFmt)
	count := map[string]map[string]int{}
	rowTot := map[string]int{}
	colTot := map[string]int{}
	grand := 0
	for _, r := range src.Rows {
		rv, cv := src.Get(r, rowVar), src.Get(r, colVar)
		if rv.IsMissing() || cv.IsMissing() {
			continue
		}
		rk, ck := rowFmt(rv), colFmt(cv)
		if count[rk] == nil {
			count[rk] = map[string]int{}
		}
		count[rk][ck]++
		rowTot[rk]++
		colTot[ck]++
		grand++
	}
	if grand == 0 || len(rowCats) < 2 || len(colCats) < 2 {
		return 0, 0, 1
	}
	for _, rc := range rowCats {
		for _, cc := range colCats {
			exp := float64(rowTot[rc.label]) * float64(colTot[cc.label]) / float64(grand)
			if exp == 0 {
				continue
			}
			obs := float64(count[rc.label][cc.label])
			d := obs - exp
			stat += d * d / exp
		}
	}
	df = (len(rowCats) - 1) * (len(colCats) - 1)
	p = chiSquareSF(stat, df)
	return stat, df, p
}

// renderChiSquare returns the chi-square statistic block appended after a two-way
// table when the `chisq` option is given.
func renderChiSquare(src *table.Dataset, rowVar, colVar string, rowFmt, colFmt func(table.Value) string) string {
	stat, df, p := chiSquareStat(src, rowVar, colVar, rowFmt, colFmt)
	return fmt.Sprintf("Statistics for Table of %s by %s\n\nChi-Square  DF=%d  Value=%.4f  Prob=%.4f\n",
		rowVar, colVar, df, stat, p)
}

// renderChiSquareHTML renders the chi-square statistic as a small styled table.
func renderChiSquareHTML(src *table.Dataset, rowVar, colVar string, rowFmt, colFmt func(table.Value) string) string {
	stat, df, p := chiSquareStat(src, rowVar, colVar, rowFmt, colFmt)
	var b strings.Builder
	b.WriteString(`<table style="` + htmlTableStyle + `">`)
	fmt.Fprintf(&b, `<caption style="%s">Statistics for Table of %s by %s</caption>`,
		htmlCaptionStyle, html.EscapeString(rowVar), html.EscapeString(colVar))
	b.WriteString(`<thead><tr>`)
	b.WriteString(`<th style="` + htmlThStyle + htmlTextStyle + `">Statistic</th>`)
	b.WriteString(`<th style="` + htmlThStyle + htmlNumStyle + `">DF</th>`)
	b.WriteString(`<th style="` + htmlThStyle + htmlNumStyle + `">Value</th>`)
	b.WriteString(`<th style="` + htmlThStyle + htmlNumStyle + `">Prob</th>`)
	b.WriteString(`</tr></thead><tbody><tr>`)
	b.WriteString(`<td style="` + htmlTdStyle + htmlTextStyle + `">Chi-Square</td>`)
	fmt.Fprintf(&b, `<td style="%s%s">%d</td>`, htmlTdStyle, htmlNumStyle, df)
	fmt.Fprintf(&b, `<td style="%s%s">%.4f</td>`, htmlTdStyle, htmlNumStyle, stat)
	fmt.Fprintf(&b, `<td style="%s%s">%.4f</td>`, htmlTdStyle, htmlNumStyle, p)
	b.WriteString(`</tr></tbody></table>`)
	return b.String()
}

// chiSquareSF returns the right-tail probability Pr(X > x) for a chi-square
// distribution with df degrees of freedom, i.e. the regularized upper incomplete
// gamma Q(df/2, x/2).
func chiSquareSF(x float64, df int) float64 {
	if x <= 0 || df <= 0 {
		return 1
	}
	return gammaQ(float64(df)/2, x/2)
}

// gammaP/gammaQ are the regularized lower/upper incomplete gamma functions,
// computed by series (gammaP, for x < a+1) or continued fraction (gammaQ),
// following the standard Numerical Recipes formulation.
func gammaP(a, x float64) float64 {
	if x < 0 || a <= 0 {
		return 0
	}
	if x < a+1 {
		// Series representation.
		ap := a
		sum := 1.0 / a
		del := sum
		for n := 0; n < 200; n++ {
			ap++
			del *= x / ap
			sum += del
			if math.Abs(del) < math.Abs(sum)*1e-15 {
				break
			}
		}
		lg, _ := math.Lgamma(a)
		return sum * math.Exp(-x+a*math.Log(x)-lg)
	}
	return 1 - gammaQ(a, x)
}

func gammaQ(a, x float64) float64 {
	if x < a+1 {
		return 1 - gammaP(a, x)
	}
	// Continued-fraction representation (Lentz's method).
	const tiny = 1e-30
	b := x + 1 - a
	c := 1 / tiny
	d := 1 / b
	h := d
	for i := 1; i < 200; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = b + an/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < 1e-15 {
			break
		}
	}
	lg, _ := math.Lgamma(a)
	return math.Exp(-x+a*math.Log(x)-lg) * h
}

func pctVal(n, total int) table.Value {
	if total == 0 {
		return table.MissingNum()
	}
	return table.Num(100 * float64(n) / float64(total))
}

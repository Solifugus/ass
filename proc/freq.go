package proc

import (
	"fmt"
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

	var requests [][]string
	for _, s := range step.Body {
		switch st := s.(type) {
		case *ast.TablesStatement:
			requests = append(requests, st.Requests...)
		case *ast.VarStatement:
			for _, v := range st.Vars {
				requests = append(requests, []string{v})
			}
		}
	}
	if len(requests) == 0 {
		logger.Error("PROC FREQ: a TABLES statement is required.")
		return nil
	}

	for _, req := range requests {
		switch len(req) {
		case 0:
			continue
		case 1:
			fmt.Print(renderListing(buildFreqResult(src, req[0]), printOptions{}))
			fmt.Println()
		default:
			// Two (or more) variables: cross-tabulate the first two.
			fmt.Print(renderCrossTab(src, req[0], req[1]))
			fmt.Println()
		}
	}
	return nil
}

// sortedDistinct returns the non-missing distinct values of variable v in src,
// in SAS sort order (ascending by Value.Compare).
func sortedDistinct(src *table.Dataset, v string) []table.Value {
	seen := map[string]bool{}
	var vals []table.Value
	for _, r := range src.Rows {
		val := src.Get(r, v)
		if val.IsMissing() {
			continue
		}
		k := val.Display()
		if !seen[k] {
			seen[k] = true
			vals = append(vals, val)
		}
	}
	sort.Slice(vals, func(i, j int) bool { return vals[i].Compare(vals[j]) < 0 })
	return vals
}

// renderCrossTab renders a SAS-style two-way frequency cross-tabulation of
// rowVar by colVar. Each interior cell stacks Frequency, Percent (of grand
// total), Row Pct, and Col Pct; the right and bottom margins carry row/column
// totals (Frequency and Percent). Missing values in either variable are
// excluded.
func renderCrossTab(src *table.Dataset, rowVar, colVar string) string {
	rowVals := sortedDistinct(src, rowVar)
	colVals := sortedDistinct(src, colVar)

	count := map[string]map[string]int{}
	rowTot := map[string]int{}
	colTot := map[string]int{}
	grand := 0
	for _, r := range src.Rows {
		rv, cv := src.Get(r, rowVar), src.Get(r, colVar)
		if rv.IsMissing() || cv.IsMissing() {
			continue
		}
		rk, ck := rv.Display(), cv.Display()
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

	// Column headers: each colVal, then "Total". The left stub holds the legend
	// labels and the row-variable values.
	headers := make([]string, 0, len(colVals)+1)
	for _, cv := range colVals {
		headers = append(headers, cv.Display())
	}
	headers = append(headers, "Total")

	legend := []string{"Frequency", "Percent", "Row Pct", "Col Pct"}
	stubW := len(rowVar)
	for _, l := range legend {
		if len(l) > stubW {
			stubW = len(l)
		}
	}
	for _, rv := range rowVals {
		if w := len(rv.Display()); w > stubW {
			stubW = w
		}
	}
	if len("Total") > stubW {
		stubW = len("Total")
	}

	// Pre-compute every data column's content width.
	colW := make([]int, len(headers))
	for i, h := range headers {
		colW[i] = len(h)
	}
	widen := func(i int, s string) {
		if len(s) > colW[i] {
			colW[i] = len(s)
		}
	}
	for _, rv := range rowVals {
		rk := rv.Display()
		for j, cv := range colVals {
			ck := cv.Display()
			n := count[rk][ck]
			widen(j, fmt.Sprintf("%d", n))
			widen(j, pct(n, grand))
			widen(j, pct(n, rowTot[rk]))
			widen(j, pct(n, colTot[ck]))
		}
		last := len(headers) - 1
		widen(last, fmt.Sprintf("%d", rowTot[rk]))
		widen(last, pct(rowTot[rk], grand))
	}
	for j, cv := range colVals {
		ck := cv.Display()
		widen(j, fmt.Sprintf("%d", colTot[ck]))
		widen(j, pct(colTot[ck], grand))
	}
	widen(len(headers)-1, fmt.Sprintf("%d", grand))

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Table of %s by %s\n\n", rowVar, colVar))

	line := func(stub string, cells []string) {
		parts := pad(stub, stubW, false)
		for i, c := range cells {
			parts += "  " + pad(c, colW[i], true)
		}
		b.WriteString(strings.TrimRight(parts, " ") + "\n")
	}

	// Header band: the column-variable name, then a corner legend (each stat once)
	// with "Col Pct" sharing the line that carries the column value headers.
	b.WriteString(strings.Repeat(" ", stubW) + "  " + colVar + "\n")
	b.WriteString(rowVar + "\n")
	b.WriteString(legend[0] + "\n")
	b.WriteString(legend[1] + "\n")
	b.WriteString(legend[2] + "\n")
	line(legend[3], headers)
	b.WriteString("\n")

	// Body: one band of four lines per row value (freq, pct, row pct, col pct),
	// with the row value labeling the first line and the rest left blank.
	for _, rv := range rowVals {
		rk := rv.Display()
		var fr, pc, rp, cp []string
		for _, cv := range colVals {
			ck := cv.Display()
			n := count[rk][ck]
			fr = append(fr, fmt.Sprintf("%d", n))
			pc = append(pc, pct(n, grand))
			rp = append(rp, pct(n, rowTot[rk]))
			cp = append(cp, pct(n, colTot[ck]))
		}
		// Row-total margin (Frequency and Percent only).
		fr = append(fr, fmt.Sprintf("%d", rowTot[rk]))
		pc = append(pc, pct(rowTot[rk], grand))
		rp = append(rp, "")
		cp = append(cp, "")

		line(rk, fr)
		line("", pc)
		line("", rp)
		line("", cp)
		b.WriteString("\n")
	}

	// Bottom margin: column totals and the grand total.
	var ct, cpc []string
	for _, cv := range colVals {
		ck := cv.Display()
		ct = append(ct, fmt.Sprintf("%d", colTot[ck]))
		cpc = append(cpc, pct(colTot[ck], grand))
	}
	ct = append(ct, fmt.Sprintf("%d", grand))
	cpc = append(cpc, pct(grand, grand))
	line("Total", ct)
	line(legend[1], cpc)

	return b.String()
}

// buildFreqResult builds the one-way frequency table for a single variable.
func buildFreqResult(src *table.Dataset, v string) *table.Dataset {
	kind := table.Character
	for _, c := range src.Columns {
		if strings.EqualFold(c.Name, v) {
			kind = c.Kind
		}
	}

	counts := map[string]int{}
	values := map[string]table.Value{}
	total := 0
	for _, r := range src.Rows {
		val := src.Get(r, v)
		if val.IsMissing() {
			continue // SAS excludes missing from one-way tables by default
		}
		key := val.Display()
		if _, seen := counts[key]; !seen {
			values[key] = val
		}
		counts[key]++
		total++
	}

	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return values[keys[i]].Compare(values[keys[j]]) < 0 })

	out := table.NewDataset("", "_freq_")
	out.AddColumn(table.Column{Name: v, Kind: kind})
	out.AddColumn(table.Column{Name: "Frequency", Kind: table.Numeric})
	out.AddColumn(table.Column{Name: "Percent", Kind: table.Numeric, Format: "5.1"})
	out.AddColumn(table.Column{Name: "CumFreq", Kind: table.Numeric})
	out.AddColumn(table.Column{Name: "CumPercent", Kind: table.Numeric, Format: "5.1"})

	cum := 0
	for _, k := range keys {
		cum += counts[k]
		row := table.Row{
			strings.ToLower(v): values[k],
			"frequency":        table.Num(float64(counts[k])),
			"percent":          pctVal(counts[k], total),
			"cumfreq":          table.Num(float64(cum)),
			"cumpercent":       pctVal(cum, total),
		}
		out.AppendRow(row)
	}
	return out
}

func pctVal(n, total int) table.Value {
	if total == 0 {
		return table.MissingNum()
	}
	return table.Num(100 * float64(n) / float64(total))
}

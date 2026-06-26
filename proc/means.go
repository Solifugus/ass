package proc

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() {
	Register("means", meansProc{})
	Register("summary", meansProc{})
}

// meansProc implements PROC MEANS / PROC SUMMARY: descriptive statistics for the
// analysis variables, optionally grouped by CLASS (or BY) variables. Output is a
// listing in the ASS format: one row per (group, analysis variable). The
// requested statistics (and their order) come from the PROC statement keywords
// (n, mean, std/stddev, min, max, sum); with none given, SAS's default set
// N Mean StdDev Min Max is used. `maxdec=k` fixes the displayed decimal places.
type meansProc struct{}

// meanStat is one selectable statistic: a canonical id (also the lowercased row
// key), a display header, and how to compute its value from a stats accumulator.
type meanStat struct {
	id    string
	head  string
	value func(stats) table.Value
}

// meansStat maps a PROC MEANS keyword to its statistic, or reports ok=false if
// the keyword is not a recognized (implemented) statistic.
func meansStat(kw string) (meanStat, bool) {
	switch kw {
	case "n":
		return meanStat{"n", "N", func(s stats) table.Value { return table.Num(float64(s.n)) }}, true
	case "mean":
		return meanStat{"mean", "Mean", func(s stats) table.Value { return s.meanVal() }}, true
	case "std", "stddev":
		return meanStat{"stddev", "StdDev", func(s stats) table.Value { return s.stdVal() }}, true
	case "min":
		return meanStat{"min", "Min", func(s stats) table.Value { return s.minVal() }}, true
	case "max":
		return meanStat{"max", "Max", func(s stats) table.Value { return s.maxVal() }}, true
	case "sum":
		return meanStat{"sum", "Sum", func(s stats) table.Value { return s.sumVal() }}, true
	}
	return meanStat{}, false
}

// defaultMeanStats is SAS's default statistic set when none are requested.
func defaultMeanStats() []meanStat {
	var out []meanStat
	for _, kw := range []string{"n", "mean", "stddev", "min", "max"} {
		st, _ := meansStat(kw)
		out = append(out, st)
	}
	return out
}

func (meansProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	src, ok := lib.Get(step.Data)
	if !ok {
		logger.Error("PROC MEANS: data set %s not found.", strings.ToUpper(step.Data))
		return nil
	}

	var analysisVars, classVars []string
	var outStmt *ast.MeansOutputStatement
	procFormats := map[string]string{}
	for _, s := range step.Body {
		switch st := s.(type) {
		case *ast.VarStatement:
			analysisVars = append(analysisVars, st.Vars...)
		case *ast.ClassStatement:
			classVars = append(classVars, st.Vars...)
		case *ast.ByStatement:
			classVars = append(classVars, st.Vars...)
		case *ast.FormatStatement:
			for k, v := range st.Formats {
				procFormats[strings.ToLower(k)] = v
			}
		case *ast.MeansOutputStatement:
			outStmt = st
		}
	}
	if len(analysisVars) == 0 {
		analysisVars = numericColumns(src)
	}

	// Statistic keywords (and their order) and maxdec= come from the PROC options.
	var selected []meanStat
	seen := map[string]bool{}
	maxdec := -1
	for _, o := range step.Options {
		if o.Name == "maxdec" {
			if k, err := strconv.Atoi(o.Value); err == nil && k >= 0 {
				maxdec = k
			}
			continue
		}
		if st, ok := meansStat(o.Name); ok && !seen[st.id] {
			selected = append(selected, st)
			seen[st.id] = true
		}
	}
	if len(selected) == 0 {
		selected = defaultMeanStats()
	}

	result := buildMeansResult(src, analysisVars, classVars, selected, maxdec, lib.Formats, procFormats)
	emitTitles(logger, lib.TitleLines())
	emitListing(logger, result, printOptions{}, "Summary Statistics")
	emitFootnotes(logger, lib.FootnoteLines())

	// `output out=<name> <stat>=<vars>` writes the statistics to a dataset.
	if outStmt != nil && outStmt.Out != "" {
		outDS := buildMeansOut(src, analysisVars, classVars, outStmt, lib.Formats, procFormats)
		if err := lib.Store(outStmt.Out, outDS); err != nil {
			logger.Error("PROC MEANS: %v", err)
		} else {
			logger.Note("The data set WORK.%s has %d observations and %d variables.",
				strings.ToUpper(datasetName(outStmt.Out)), outDS.NObs(), len(outDS.Columns))
		}
	}
	return nil
}

// buildMeansOut builds the PROC MEANS `output out=` dataset: the class variables,
// then _TYPE_ and _FREQ_, then one column per requested statistic output variable
// (positionally matched to the analysis variables). One row per class group (the
// detail/nway level).
func buildMeansOut(src *table.Dataset, analysisVars, classVars []string, outStmt *ast.MeansOutputStatement, cat *table.FormatCatalog, procFormats map[string]string) *table.Dataset {
	fmts := make([]func(table.Value) string, len(classVars))
	formatted := make([]bool, len(classVars))
	for i, cv := range classVars {
		fmts[i] = freqFormatter(src, cat, procFormats, cv)
		formatted[i] = varFormatSpec(src, procFormats, cv) != ""
	}

	out := table.NewDataset("", datasetName(outStmt.Out))
	for i, cv := range classVars {
		kind := table.Numeric
		for _, c := range src.Columns {
			if strings.EqualFold(c.Name, cv) {
				kind = c.Kind
			}
		}
		if formatted[i] {
			kind = table.Character
		}
		out.AddColumn(table.Column{Name: cv, Kind: kind})
	}
	out.AddColumn(table.Column{Name: "_TYPE_", Kind: table.Numeric})
	out.AddColumn(table.Column{Name: "_FREQ_", Kind: table.Numeric})

	// Statistic output columns, in clause then name order; each maps positionally
	// to an analysis variable.
	type statCol struct{ name, av, stat string }
	var statCols []statCol
	for _, sc := range outStmt.Stats {
		for i, nm := range sc.Names {
			if i >= len(analysisVars) {
				break
			}
			statCols = append(statCols, statCol{name: nm, av: analysisVars[i], stat: sc.Stat})
			out.AddColumn(table.Column{Name: nm, Kind: table.Numeric})
		}
	}

	typeBits := 0
	if n := len(classVars); n > 0 {
		typeBits = (1 << uint(n)) - 1 // 0 with no class vars
	}
	typeVal := float64(typeBits)
	groups, order := groupRows(src, classVars, fmts)
	for _, key := range order {
		rows := groups[key]
		row := table.Row{}
		for i, cv := range classVars {
			if formatted[i] {
				row[strings.ToLower(cv)] = table.Char(fmts[i](src.Get(rows[0], cv)))
			} else {
				row[strings.ToLower(cv)] = src.Get(rows[0], cv)
			}
		}
		row["_type_"] = table.Num(typeVal)
		row["_freq_"] = table.Num(float64(len(rows)))
		for _, sc := range statCols {
			if ms, ok := meansStat(sc.stat); ok {
				row[strings.ToLower(sc.name)] = ms.value(computeStats(src, rows, sc.av))
			}
		}
		out.AppendRow(row)
	}
	return out
}

// buildMeansResult computes the statistics table. CLASS variables are grouped
// (and displayed) by their user/built-in formatted value when a format applies,
// matching SAS — so a user VALUE format collapses underlying values into one
// class level.
func buildMeansResult(src *table.Dataset, analysisVars, classVars []string, selected []meanStat, maxdec int, cat *table.FormatCatalog, procFormats map[string]string) *table.Dataset {
	fmts := make([]func(table.Value) string, len(classVars))
	formatted := make([]bool, len(classVars))
	for i, cv := range classVars {
		fmts[i] = freqFormatter(src, cat, procFormats, cv)
		formatted[i] = varFormatSpec(src, procFormats, cv) != ""
	}

	// maxdec= attaches a w.d display format to the float statistics (N stays an
	// integer count). A generous width avoids best-fallback; the listing trims it.
	statFmt := ""
	if maxdec >= 0 {
		statFmt = fmt.Sprintf("%d.%d", maxdec+12, maxdec)
	}

	out := table.NewDataset("", "_means_")
	for i, cv := range classVars {
		kind := table.Numeric
		for _, c := range src.Columns {
			if strings.EqualFold(c.Name, cv) {
				kind = c.Kind
			}
		}
		if formatted[i] {
			kind = table.Character
		}
		out.AddColumn(table.Column{Name: cv, Kind: kind})
	}
	out.AddColumn(table.Column{Name: "Variable", Kind: table.Character})
	for _, ms := range selected {
		col := table.Column{Name: ms.head, Kind: table.Numeric}
		if statFmt != "" && ms.id != "n" {
			col.Format = statFmt
		}
		out.AddColumn(col)
	}

	groups, order := groupRows(src, classVars, fmts)
	for _, key := range order {
		rows := groups[key]
		for _, v := range analysisVars {
			st := computeStats(src, rows, v)
			row := table.Row{}
			for i, cv := range classVars {
				if formatted[i] {
					row[strings.ToLower(cv)] = table.Char(fmts[i](src.Get(rows[0], cv)))
				} else {
					row[strings.ToLower(cv)] = src.Get(rows[0], cv)
				}
			}
			row["variable"] = table.Char(v)
			for _, ms := range selected {
				row[ms.id] = ms.value(st)
			}
			out.AppendRow(row)
		}
	}
	return out
}

type stats struct {
	n        int
	sum      float64
	min, max float64
	sumsq    float64
}

func computeStats(ds *table.Dataset, rows []table.Row, v string) stats {
	var s stats
	for _, r := range rows {
		val := ds.Get(r, v)
		if val.IsMissing() || val.Kind != table.Numeric {
			continue
		}
		x := val.Num
		if s.n == 0 || x < s.min {
			s.min = x
		}
		if s.n == 0 || x > s.max {
			s.max = x
		}
		s.n++
		s.sum += x
		s.sumsq += x * x
	}
	return s
}

func (s stats) meanVal() table.Value {
	if s.n == 0 {
		return table.MissingNum()
	}
	return table.Num(s.sum / float64(s.n))
}

func (s stats) stdVal() table.Value {
	if s.n < 2 {
		return table.MissingNum()
	}
	mean := s.sum / float64(s.n)
	variance := (s.sumsq - float64(s.n)*mean*mean) / float64(s.n-1)
	if variance < 0 {
		variance = 0 // guard against tiny negative from rounding
	}
	return table.Num(math.Sqrt(variance))
}

func (s stats) minVal() table.Value {
	if s.n == 0 {
		return table.MissingNum()
	}
	return table.Num(s.min)
}

func (s stats) maxVal() table.Value {
	if s.n == 0 {
		return table.MissingNum()
	}
	return table.Num(s.max)
}

func (s stats) sumVal() table.Value {
	if s.n == 0 {
		return table.MissingNum()
	}
	return table.Num(s.sum)
}

// groupRows groups a dataset's rows by the class variables, returning the groups
// and the keys in sorted order. With no class variables, a single group keyed ""
// holds every row.
func groupRows(ds *table.Dataset, classVars []string, fmts []func(table.Value) string) (map[string][]table.Row, []string) {
	groups := map[string][]table.Row{}
	var order []string
	for _, r := range ds.Rows {
		key := ""
		if len(classVars) > 0 {
			parts := make([]string, len(classVars))
			for i, c := range classVars {
				if fmts != nil && fmts[i] != nil {
					parts[i] = fmts[i](ds.Get(r, c))
				} else {
					parts[i] = ds.Get(r, c).Display()
				}
			}
			key = strings.Join(parts, "\x00")
		}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], r)
	}
	if len(classVars) > 0 {
		sort.Slice(order, func(i, j int) bool {
			ri, rj := groups[order[i]][0], groups[order[j]][0]
			for _, c := range classVars {
				if cmp := ds.Get(ri, c).Compare(ds.Get(rj, c)); cmp != 0 {
					return cmp < 0
				}
			}
			return false
		})
	}
	return groups, order
}

// numericColumns returns the names of a dataset's numeric columns.
func numericColumns(ds *table.Dataset) []string {
	var out []string
	for _, c := range ds.Columns {
		if c.Kind == table.Numeric {
			out = append(out, c.Name)
		}
	}
	return out
}

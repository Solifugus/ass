package proc

import (
	"math"
	"sort"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() {
	Register("means", meansProc{})
	Register("summary", meansProc{})
}

// meansProc implements PROC MEANS / PROC SUMMARY: descriptive statistics
// (N, Mean, StdDev, Min, Max) for the analysis variables, optionally grouped by
// CLASS (or BY) variables. Output is a listing in the ASS format: one row per
// (group, analysis variable).
type meansProc struct{}

func (meansProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	src, ok := lib.Get(step.Data)
	if !ok {
		logger.Error("PROC MEANS: data set %s not found.", strings.ToUpper(step.Data))
		return nil
	}

	var analysisVars, classVars []string
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
		}
	}
	if len(analysisVars) == 0 {
		analysisVars = numericColumns(src)
	}

	result := buildMeansResult(src, analysisVars, classVars, lib.Formats, procFormats)
	emitListing(logger, result, printOptions{}, "Summary Statistics")
	return nil
}

// buildMeansResult computes the statistics table. CLASS variables are grouped
// (and displayed) by their user/built-in formatted value when a format applies,
// matching SAS — so a user VALUE format collapses underlying values into one
// class level.
func buildMeansResult(src *table.Dataset, analysisVars, classVars []string, cat *table.FormatCatalog, procFormats map[string]string) *table.Dataset {
	fmts := make([]func(table.Value) string, len(classVars))
	formatted := make([]bool, len(classVars))
	for i, cv := range classVars {
		fmts[i] = freqFormatter(src, cat, procFormats, cv)
		formatted[i] = varFormatSpec(src, procFormats, cv) != ""
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
	out.AddColumn(table.Column{Name: "N", Kind: table.Numeric})
	out.AddColumn(table.Column{Name: "Mean", Kind: table.Numeric})
	out.AddColumn(table.Column{Name: "StdDev", Kind: table.Numeric})
	out.AddColumn(table.Column{Name: "Min", Kind: table.Numeric})
	out.AddColumn(table.Column{Name: "Max", Kind: table.Numeric})

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
			row["n"] = table.Num(float64(st.n))
			row["mean"] = st.meanVal()
			row["stddev"] = st.stdVal()
			row["min"] = st.minVal()
			row["max"] = st.maxVal()
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

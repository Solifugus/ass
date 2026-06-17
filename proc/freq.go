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

	var vars []string
	for _, s := range step.Body {
		switch st := s.(type) {
		case *ast.TablesStatement:
			vars = append(vars, st.Vars...)
		case *ast.VarStatement:
			vars = append(vars, st.Vars...)
		}
	}
	if len(vars) == 0 {
		logger.Error("PROC FREQ: a TABLES statement is required.")
		return nil
	}

	for _, v := range vars {
		fmt.Print(renderListing(buildFreqResult(src, v), printOptions{}))
		fmt.Println()
	}
	return nil
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

package proc

import (
	"sort"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() { Register("sort", sortProc{}) }

// sortProc implements PROC SORT: it orders a dataset by one or more BY keys
// (each optionally descending), writing the result in place or to an OUT=
// dataset, and can drop duplicate-key rows with NODUPKEY.
type sortProc struct{}

func (sortProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	src, ok := lib.Get(step.Data)
	if !ok {
		logger.Error("PROC SORT: data set %s not found.", strings.ToUpper(step.Data))
		return nil
	}

	by := sortKeys(step.Body)
	if len(by) == 0 {
		logger.Error("PROC SORT: a BY statement is required.")
		return nil
	}

	var out string
	var nodupkey bool
	for _, o := range step.Options {
		switch strings.ToLower(o.Name) {
		case "out":
			out = o.Value
		case "nodupkey":
			nodupkey = true
		}
	}

	// Sort a copy so an OUT= sort leaves the source untouched.
	rows := append([]table.Row(nil), src.Rows...)
	sort.SliceStable(rows, func(i, j int) bool {
		for _, k := range by {
			c := src.Get(rows[i], k.name).Compare(src.Get(rows[j], k.name))
			if c != 0 {
				if k.descending {
					return c > 0
				}
				return c < 0
			}
		}
		return false // equal keys: stable order preserves input sequence
	})

	if nodupkey {
		rows = dropDupKeys(src, rows, by)
	}

	// Destination: in place (DATA=) when no OUT=, otherwise the OUT= name. A
	// libref-qualified OUT= (e.g. out=db.sorted) is written to that LIBNAME engine
	// via lib.Store; everything else lands in WORK.
	dest := step.Data
	target := src
	if out != "" {
		dest = out
		target = table.NewDataset("", datasetName(out))
		target.Columns = src.Columns // share immutable column metadata
	}
	target.Rows = rows
	if err := lib.Store(dest, target); err != nil {
		logger.Error("PROC SORT: %v", err)
		return nil
	}
	logger.Note("The data set %s.%s has %d observations and %d variables.",
		strings.ToUpper(target.Lib), strings.ToUpper(target.Name), target.NObs(), len(target.Columns))
	return nil
}

// sortKey is one BY variable and its direction.
type sortKey struct {
	name       string
	descending bool
}

// sortKeys extracts the BY variables (and their descending flags) from the step
// body.
func sortKeys(stmts []ast.Statement) []sortKey {
	var keys []sortKey
	for _, s := range stmts {
		by, ok := s.(*ast.ByStatement)
		if !ok {
			continue
		}
		for i, v := range by.Vars {
			desc := i < len(by.Descending) && by.Descending[i]
			keys = append(keys, sortKey{name: v, descending: desc})
		}
	}
	return keys
}

// dropDupKeys keeps only the first row of each run of equal BY-key values in an
// already-sorted slice (NODUPKEY semantics).
func dropDupKeys(ds *table.Dataset, rows []table.Row, by []sortKey) []table.Row {
	var out []table.Row
	for i, r := range rows {
		if i == 0 || keyDiffers(ds, rows[i-1], r, by) {
			out = append(out, r)
		}
	}
	return out
}

// keyDiffers reports whether two rows differ on any BY-key variable.
func keyDiffers(ds *table.Dataset, a, b table.Row, by []sortKey) bool {
	for _, k := range by {
		if ds.Get(a, k.name).Compare(ds.Get(b, k.name)) != 0 {
			return true
		}
	}
	return false
}

// datasetName strips a library qualifier from a dataset reference.
func datasetName(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

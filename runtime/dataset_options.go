package runtime

import (
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/table"
)

// applyDatasetOptions returns a view of src with dataset options applied: WHERE
// filters rows, KEEP/DROP select columns, and RENAME renames them. The source is
// never mutated; a new dataset is returned. When opts is empty, src is returned
// unchanged. Following SAS, on input KEEP/DROP and WHERE reference the original
// variable names and RENAME is applied afterward.
func applyDatasetOptions(src *table.Dataset, opts *ast.DatasetOptions) (*table.Dataset, error) {
	if opts.IsEmpty() {
		return src, nil
	}

	// 0. Select the observation range by position (FIRSTOBS=/OBS=) before any
	// WHERE filtering, mirroring SAS: OBS= is the number of the last observation
	// processed (not a row count), FIRSTOBS= the first.
	rows := src.Rows
	if opts.FirstObs > 0 || opts.Obs > 0 {
		lo := 0
		if opts.FirstObs > 0 {
			lo = opts.FirstObs - 1
		}
		hi := len(rows)
		if opts.Obs > 0 && opts.Obs < hi {
			hi = opts.Obs
		}
		if lo > len(rows) {
			lo = len(rows)
		}
		if hi < lo {
			hi = lo
		}
		rows = rows[lo:hi]
	}

	// 1. Filter rows by WHERE (evaluated against original names).
	if opts.Where != nil {
		kept := make([]table.Row, 0, len(rows))
		for _, r := range rows {
			pdv := NewPDV()
			for _, c := range src.Columns {
				pdv.Set(c.Name, src.Get(r, c.Name))
			}
			v, err := Eval(opts.Where, pdv)
			if err != nil {
				return nil, err
			}
			if truthy(v) {
				kept = append(kept, r)
			}
		}
		rows = kept
	}

	// 2. Decide which columns survive KEEP/DROP (original names).
	keepSet := lowerSet(opts.Keep)
	dropSet := lowerSet(opts.Drop)
	survives := func(name string) bool {
		ln := strings.ToLower(name)
		if len(keepSet) > 0 {
			return keepSet[ln]
		}
		if len(dropSet) > 0 {
			return !dropSet[ln]
		}
		return true
	}

	// 3. Build the output dataset with renamed columns.
	out := table.NewDataset(src.Lib, src.Name)
	rename := map[string]string{}
	for k, v := range opts.Rename {
		rename[strings.ToLower(k)] = v
	}
	newName := func(name string) string {
		if nn, ok := rename[strings.ToLower(name)]; ok {
			return nn
		}
		return name
	}
	for _, c := range src.Columns {
		if !survives(c.Name) {
			continue
		}
		nc := c
		nc.Name = newName(c.Name)
		out.AddColumn(nc)
	}

	// 4. Copy the surviving rows under the (possibly renamed) keys.
	for _, r := range rows {
		nr := make(table.Row, len(out.Columns))
		for _, c := range src.Columns {
			if !survives(c.Name) {
				continue
			}
			nr[strings.ToLower(newName(c.Name))] = src.Get(r, c.Name)
		}
		out.AppendRow(nr)
	}
	return out, nil
}

// lowerSet returns a set of the lowercased names (nil if empty).
func lowerSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[strings.ToLower(n)] = true
	}
	return m
}

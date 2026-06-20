package runtime

import (
	"sort"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/table"
)

// findMerge returns the MERGE statement in a body, or nil.
func findMerge(stmts []ast.Statement) *ast.MergeStatement {
	for _, s := range stmts {
		if m, ok := s.(*ast.MergeStatement); ok {
			return m
		}
	}
	return nil
}

// byVarsOf returns the BY variables declared in a body (empty if none).
func byVarsOf(stmts []ast.Statement) []string {
	for _, s := range stmts {
		if by, ok := s.(*ast.ByStatement); ok {
			return by.Vars
		}
	}
	return nil
}

// mergeSource is one input dataset of a MERGE, with its rows grouped by BY key.
type mergeSource struct {
	ref    ast.DatasetRef
	ds     *table.Dataset
	groups map[string][]table.Row
}

// buildMerge precomputes the combined output rows of a match-merge. Inputs are
// assumed sorted by the BY variables. Within each BY group it emits
// max(group sizes) rows; a source with fewer rows holds its last values, and its
// in= flag is 1 only on rows it freshly contributes. BY variables are taken from
// whichever source has the group (never overwritten with missing). Automatic
// first./last.<byvar> are set at group boundaries.
func (d *dataStep) buildMerge(m *ast.MergeStatement, byVars []string) error {
	d.byVars = byVars
	d.inVars = make(map[string]bool)

	byset := make(map[string]bool)
	for _, v := range byVars {
		byset[strings.ToLower(v)] = true
	}

	// Load sources, group their rows by BY key, and declare output columns in
	// merge order (so column layout is dataset1 then dataset2's new vars).
	var sources []mergeSource
	for _, ref := range m.Refs {
		raw, ok, err := d.lib.Resolve(ref.Name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		ds, err := applyDatasetOptions(raw, ref.Options)
		if err != nil {
			return err
		}
		d.recordSourceCols(ds)
		src := mergeSource{ref: ref, ds: ds, groups: map[string][]table.Row{}}
		for _, r := range ds.Rows {
			k := byKey(ds, r, byVars)
			src.groups[k] = append(src.groups[k], r)
		}
		sources = append(sources, src)
		for _, c := range ds.Columns {
			d.pdv.Declare(c.Name, c.Kind)
		}
		if ref.In != "" {
			d.inVars[strings.ToLower(ref.In)] = true
		}
	}

	// Collect distinct keys with a representative row, then sort by BY values.
	type keyRep struct {
		key string
		row table.Row
		ds  *table.Dataset
	}
	seen := map[string]bool{}
	var reps []keyRep
	for _, src := range sources {
		for k, rows := range src.groups {
			if !seen[k] {
				seen[k] = true
				reps = append(reps, keyRep{key: k, row: rows[0], ds: src.ds})
			}
		}
	}
	sort.SliceStable(reps, func(i, j int) bool {
		for _, v := range byVars {
			c := reps[i].ds.Get(reps[i].row, v).Compare(reps[j].ds.Get(reps[j].row, v))
			if c != 0 {
				return c < 0
			}
		}
		return false
	})

	// Emit rows per group.
	for _, rep := range reps {
		nrows := 0
		for _, src := range sources {
			if n := len(src.groups[rep.key]); n > nrows {
				nrows = n
			}
		}
		groupStart := len(d.mergeRows)
		for i := 0; i < nrows; i++ {
			row := make(table.Row)
			// BY variables from the representative (stable across the group).
			for _, v := range byVars {
				row[strings.ToLower(v)] = rep.ds.Get(rep.row, v)
			}
			for _, src := range sources {
				grp := src.groups[rep.key]
				var srcRow table.Row
				contributed := 0.0
				switch {
				case i < len(grp):
					srcRow = grp[i]
					contributed = 1
				case len(grp) > 0:
					srcRow = grp[len(grp)-1] // hold last values
				}
				for _, c := range src.ds.Columns {
					if byset[strings.ToLower(c.Name)] {
						continue // BY vars already set
					}
					if srcRow != nil {
						row[strings.ToLower(c.Name)] = src.ds.Get(srcRow, c.Name)
					} else {
						row[strings.ToLower(c.Name)] = typedMissing(c.Kind)
					}
				}
				if src.ref.In != "" {
					row[strings.ToLower(src.ref.In)] = table.Num(contributed)
				}
			}
			d.mergeRows = append(d.mergeRows, row)
		}
		// first./last. flags for this group's output rows.
		for idx := groupStart; idx < len(d.mergeRows); idx++ {
			first, last := 0.0, 0.0
			if idx == groupStart {
				first = 1
			}
			if idx == len(d.mergeRows)-1 {
				last = 1
			}
			for _, v := range byVars {
				d.mergeRows[idx]["first."+strings.ToLower(v)] = table.Num(first)
				d.mergeRows[idx]["last."+strings.ToLower(v)] = table.Num(last)
			}
		}
	}
	return nil
}

// byKey builds a canonical grouping key from a row's BY-variable values.
func byKey(ds *table.Dataset, r table.Row, byVars []string) string {
	parts := make([]string, len(byVars))
	for i, v := range byVars {
		parts[i] = ds.Get(r, v).Display()
	}
	return strings.Join(parts, "\x00")
}

func typedMissing(k table.Kind) table.Value {
	if k == table.Character {
		return table.MissingChar()
	}
	return table.MissingNum()
}

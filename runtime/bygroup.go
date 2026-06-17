package runtime

import "github.com/solifugus/ass/table"

// ByFlags holds the SAS BY-group boundary indicators for one observation: per
// BY variable (in priority order), whether the row is the first and/or last of
// its group.
type ByFlags struct {
	First []bool
	Last  []bool
}

// ComputeByGroups computes the first./last. flags for every row of a dataset
// that is already sorted by the given BY variables. For BY variables v1..vk (in
// priority order):
//
//   - first.vi is true when vi (or any higher-priority variable) changes from
//     the previous row — and always for the first row.
//   - last.vi is true when vi (or any higher-priority variable) changes in the
//     next row — and always for the last row.
//
// Once a higher-priority variable starts (or ends) a group, every lower-priority
// variable does too, so the flags cascade. This is the data the Phase 10 DATA
// step runtime exposes as the automatic first./last. variables.
func ComputeByGroups(ds *table.Dataset, byVars []string) []ByFlags {
	n := ds.NObs()
	flags := make([]ByFlags, n)
	nv := len(byVars)

	for i := 0; i < n; i++ {
		first := make([]bool, nv)
		last := make([]bool, nv)

		// A group for variable k starts here if everything from index d onward
		// changed relative to the previous row (d = first changed variable).
		d := 0
		if i > 0 {
			d = changeIndex(ds, byVars, ds.Rows[i-1], ds.Rows[i])
		}
		for k := 0; k < nv; k++ {
			first[k] = k >= d
		}

		dl := 0
		if i < n-1 {
			dl = changeIndex(ds, byVars, ds.Rows[i], ds.Rows[i+1])
		}
		for k := 0; k < nv; k++ {
			last[k] = k >= dl
		}

		flags[i] = ByFlags{First: first, Last: last}
	}
	return flags
}

// changeIndex returns the index of the first BY variable whose value differs
// between rows a and b, or len(byVars) if they match on every BY variable.
func changeIndex(ds *table.Dataset, byVars []string, a, b table.Row) int {
	for k, v := range byVars {
		if ds.Get(a, v).Compare(ds.Get(b, v)) != 0 {
			return k
		}
	}
	return len(byVars)
}

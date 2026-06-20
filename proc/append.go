package proc

import (
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() { Register("append", appendProc{}) }

// appendProc implements PROC APPEND: it adds the observations of DATA= to the
// end of BASE=, creating BASE= if it does not yet exist. BASE= or DATA= may be a
// database libref, so an append can load a table incrementally (the engine path
// is an INSERT-only write, not a drop-and-recreate). FORCE permits appending
// when DATA= has variables BASE= lacks (they are dropped) or a variable's type
// disagrees (it is set missing in BASE=); without FORCE such a mismatch refuses
// the append, as in SAS.
type appendProc struct{}

func (appendProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	var base string
	var force bool
	for _, o := range step.Options {
		switch strings.ToLower(o.Name) {
		case "base":
			base = o.Value
		case "force":
			force = true
		case "data":
			// runProcStep already resolved DATA= into step.Data (applying any
			// dataset options / external load); ignore the raw option here.
		}
	}
	if base == "" {
		logger.Error("PROC APPEND: the BASE= data set is required.")
		return nil
	}
	if step.Data == "" {
		logger.Error("PROC APPEND: the DATA= data set is required.")
		return nil
	}

	dataDS, ok := lib.Get(step.Data)
	if !ok {
		logger.Error("PROC APPEND: data set %s not found.", strings.ToUpper(step.Data))
		return nil
	}

	baseDS, baseExists, err := lib.Resolve(base)
	if err != nil {
		logger.Error("PROC APPEND: %v", err)
		return nil
	}

	logger.Note("Appending %s to %s.", upperName(step.Data), upperName(base))

	// BASE= does not exist yet: create it as a copy of DATA= (SAS creates the
	// base data set on the first append).
	if !baseExists {
		out := table.NewDataset("", datasetName(base))
		out.Columns = dataDS.Columns
		out.Rows = append([]table.Row(nil), dataDS.Rows...)
		if err := lib.Store(base, out); err != nil {
			logger.Error("PROC APPEND: %v", err)
			return nil
		}
		logger.Note("BASE data set %s.%s did not exist; it was created from %s.",
			strings.ToUpper(out.Lib), strings.ToUpper(out.Name), upperName(step.Data))
		logger.Note("The data set %s.%s has %d observations and %d variables.",
			strings.ToUpper(out.Lib), strings.ToUpper(out.Name), out.NObs(), len(out.Columns))
		return nil
	}

	rows, extras, mismatches := alignForAppend(baseDS, dataDS)
	if (len(extras) > 0 || len(mismatches) > 0) && !force {
		for _, e := range extras {
			logger.Error("PROC APPEND: variable %s was not found on the BASE data set. (Use FORCE to drop it.)", e)
		}
		for _, m := range mismatches {
			logger.Error("PROC APPEND: variable %s does not have matching type. (Use FORCE to override.)", m)
		}
		logger.Error("PROC APPEND: %s was not appended because of differing variables. Use FORCE to force the append.", upperName(step.Data))
		return nil
	}

	// Capture the base row count before appending: for a WORK base, lib.Append
	// mutates the very dataset baseDS points to, so reading len afterward would
	// double-count.
	baseCount := len(baseDS.Rows)

	appendDS := table.NewDataset("", datasetName(base))
	appendDS.Columns = baseDS.Columns
	appendDS.Rows = rows
	if err := lib.Append(base, appendDS); err != nil {
		logger.Error("PROC APPEND: %v", err)
		return nil
	}

	logger.Note("There were %d observations read from the data set %s.",
		len(dataDS.Rows), upperName(step.Data))
	// Report the resulting BASE size (base-before + appended); cheap to compute
	// and correct for both the WORK and external paths without a reload.
	total := baseCount + len(rows)
	logger.Note("The data set %s.%s has %d observations and %d variables.",
		strings.ToUpper(appendDS.Lib), strings.ToUpper(appendDS.Name), total, len(appendDS.Columns))
	return nil
}

// alignForAppend builds the rows to append to base from data's rows, keeping only
// base's columns (each value taken by name, missing where data lacks the
// column). It reports data variables absent from base (extras, dropped) and
// variables whose type disagrees with base (mismatches, set missing) so the
// caller can require FORCE.
func alignForAppend(base, data *table.Dataset) (rows []table.Row, extras, mismatches []string) {
	baseByName := make(map[string]table.Column, len(base.Columns))
	for _, c := range base.Columns {
		baseByName[strings.ToLower(c.Name)] = c
	}
	for _, dc := range data.Columns {
		bc, ok := baseByName[strings.ToLower(dc.Name)]
		if !ok {
			extras = append(extras, dc.Name)
		} else if bc.Kind != dc.Kind {
			mismatches = append(mismatches, dc.Name)
		}
	}
	mismatchSet := make(map[string]bool, len(mismatches))
	for _, m := range mismatches {
		mismatchSet[strings.ToLower(m)] = true
	}

	rows = make([]table.Row, 0, len(data.Rows))
	for _, dr := range data.Rows {
		nr := make(table.Row, len(base.Columns))
		for _, bc := range base.Columns {
			ln := strings.ToLower(bc.Name)
			if mismatchSet[ln] {
				nr[ln] = typedMissing(bc.Kind)
				continue
			}
			if v, ok := dr[ln]; ok {
				nr[ln] = v
			} else {
				nr[ln] = typedMissing(bc.Kind)
			}
		}
		rows = append(rows, nr)
	}
	return rows, extras, mismatches
}

// typedMissing returns the missing value matching a column's kind.
func typedMissing(k table.Kind) table.Value {
	if k == table.Character {
		return table.MissingChar()
	}
	return table.MissingNum()
}

// upperName renders a (possibly qualified) dataset reference for a log NOTE,
// defaulting an unqualified name to the WORK library, as SAS logs it.
func upperName(name string) string {
	if strings.Contains(name, ".") {
		return strings.ToUpper(name)
	}
	return "WORK." + strings.ToUpper(name)
}

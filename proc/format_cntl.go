package proc

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

// PROC FORMAT CNTLOUT= / CNTLIN= — persisting user VALUE formats as a "control"
// dataset and rebuilding them from one. The control dataset uses the standard SAS
// column set (the publicly documented subset): FMTNAME, START, END, LABEL, TYPE
// ('N' numeric / 'C' character), HLO (flags: L=START is LOW, H=END is HIGH,
// O=OTHER), and SEXCL/EEXCL ('Y'/'N' exclusive bounds). PICTURE and INVALUE
// catalogs are not yet round-tripped.

// formatCntlout writes the catalog's VALUE formats to a control dataset.
func formatCntlout(lib *table.Library, name string, logger *log.Logger) error {
	ds := table.NewDataset("", name)
	for _, col := range []string{"FMTNAME", "START", "END", "LABEL", "TYPE", "HLO", "SEXCL", "EEXCL"} {
		ds.AddColumn(table.Column{Name: col, Kind: table.Character})
	}

	skipped := 0
	for _, vf := range lib.Formats.All() {
		if vf.Picture {
			skipped++
			continue // PICTURE round-trip not yet supported
		}
		typ := "N"
		if vf.Char {
			typ = "C"
		}
		fmtname := strings.ToUpper(strings.TrimPrefix(vf.Name, "$"))
		for i := range vf.Ranges {
			r := &vf.Ranges[i]
			start, end, hlo := "", "", ""
			switch {
			case r.Other:
				hlo = "O"
			default:
				if r.NoLow {
					hlo += "L"
				} else {
					start = boundStr(r.Low, vf.Char)
				}
				if r.NoHigh {
					hlo += "H"
				} else {
					end = boundStr(r.High, vf.Char)
				}
			}
			ds.AppendRow(table.Row{
				"fmtname": table.Char(fmtname),
				"start":   table.Char(start),
				"end":     table.Char(end),
				"label":   table.Char(r.Label),
				"type":    table.Char(typ),
				"hlo":     table.Char(hlo),
				"sexcl":   table.Char(ynChar(r.LowExcl)),
				"eexcl":   table.Char(ynChar(r.HighExcl)),
			})
		}
	}
	if skipped > 0 {
		logger.Note("CNTLOUT=: %d PICTURE format(s) skipped (not round-tripped).", skipped)
	}
	if err := lib.Store(name, ds); err != nil {
		return err
	}
	logger.Note("The data set WORK.%s has %d observations and %d variables.",
		strings.ToUpper(name), ds.NObs(), len(ds.Columns))
	return nil
}

// formatCntlin builds VALUE formats from a control dataset and defines them in
// the catalog. Rows are grouped into formats by (FMTNAME, TYPE) in first-seen
// order; column lookups are case-insensitive and tolerant of absent columns.
func formatCntlin(lib *table.Library, name string, logger *log.Logger) error {
	src, ok := lib.Get(name)
	if !ok {
		return fmt.Errorf("control data set %q not found", name)
	}
	byKey := map[string]*table.ValueFormat{}
	var order []*table.ValueFormat

	for _, row := range src.Rows {
		fmtname := strings.TrimSpace(src.Get(row, "fmtname").Str)
		if fmtname == "" {
			continue
		}
		char := strings.EqualFold(strings.TrimSpace(src.Get(row, "type").Str), "C")
		display := fmtname
		if char {
			display = "$" + fmtname
		}
		key := strings.ToLower(display)
		vf := byKey[key]
		if vf == nil {
			vf = &table.ValueFormat{Name: display, Char: char}
			byKey[key] = vf
			order = append(order, vf)
		}

		hlo := strings.ToUpper(src.Get(row, "hlo").Str)
		startS := strings.TrimSpace(src.Get(row, "start").Str)
		endS := strings.TrimSpace(src.Get(row, "end").Str)
		r := table.FormatRange{
			Label:    src.Get(row, "label").Str,
			Other:    strings.Contains(hlo, "O"),
			NoLow:    strings.Contains(hlo, "L"),
			NoHigh:   strings.Contains(hlo, "H"),
			LowExcl:  isYes(src.Get(row, "sexcl").Str),
			HighExcl: isYes(src.Get(row, "eexcl").Str),
		}
		if !r.Other {
			if !r.NoLow {
				r.Low = parseBound(startS, char)
			}
			if !r.NoHigh {
				if endS == "" && startS != "" {
					r.High = parseBound(startS, char) // single value: END defaults to START
				} else {
					r.High = parseBound(endS, char)
				}
			}
		}
		vf.Ranges = append(vf.Ranges, r)
	}

	for _, vf := range order {
		lib.Formats.Define(vf)
		logger.Note("Format %s has been output.", strings.ToUpper(strings.TrimPrefix(vf.Name, "$")))
	}
	return nil
}

// boundStr renders a range bound as control-dataset text: the string for a
// character format, else a compact numeric literal.
func boundStr(v table.Value, char bool) string {
	if char {
		return v.Str
	}
	if v.IsMissing() {
		return ""
	}
	return strconv.FormatFloat(v.Num, 'g', -1, 64)
}

// parseBound converts control-dataset text back to a bound Value.
func parseBound(s string, char bool) table.Value {
	if char {
		return table.Char(s)
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return table.MissingNum()
	}
	return table.Num(n)
}

func ynChar(b bool) string {
	if b {
		return "Y"
	}
	return "N"
}

func isYes(s string) bool { return strings.EqualFold(strings.TrimSpace(s), "Y") }

package runtime

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/solifugus/ass/formats"
	"github.com/solifugus/ass/table"
)

// Date/time functions. SAS represents a date as the integer number of days since
// 1960-01-01, a datetime as seconds since 1960-01-01 00:00:00, and a time as
// seconds since midnight. These functions operate on those numeric encodings,
// reusing the epoch conversions in the formats package. Missing arguments
// propagate to a missing result, as in SAS.

// today / date — the current date as a SAS day number.
func todayFn(args []table.Value) (table.Value, error) {
	if len(args) != 0 {
		return table.MissingNum(), fmt.Errorf("today expects no arguments, got %d", len(args))
	}
	return table.Num(formats.TimeToSASDate(time.Now())), nil
}

// datetime — the current datetime as a SAS datetime value.
func datetimeFn(args []table.Value) (table.Value, error) {
	if len(args) != 0 {
		return table.MissingNum(), fmt.Errorf("datetime expects no arguments, got %d", len(args))
	}
	return table.Num(formats.TimeToSASDatetime(time.Now())), nil
}

// time — the current time of day as seconds since midnight.
func timeFn(args []table.Value) (table.Value, error) {
	if len(args) != 0 {
		return table.MissingNum(), fmt.Errorf("time expects no arguments, got %d", len(args))
	}
	h, m, s := time.Now().Clock()
	return table.Num(float64(h*3600 + m*60 + s)), nil
}

// mdy(month, day, year) — the SAS date for that calendar date, or missing if the
// date is invalid (e.g. month 13 or 30FEB).
func mdyFn(args []table.Value) (table.Value, error) {
	if len(args) != 3 {
		return table.MissingNum(), fmt.Errorf("mdy expects 3 arguments, got %d", len(args))
	}
	if args[0].IsMissing() || args[1].IsMissing() || args[2].IsMissing() {
		return table.MissingNum(), nil
	}
	m, d, y := int(args[0].Num), int(args[1].Num), int(args[2].Num)
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return table.MissingNum(), nil
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	// time.Date normalizes out-of-range days (30FEB -> 01MAR); reject those.
	if t.Year() != y || int(t.Month()) != m || t.Day() != d {
		return table.MissingNum(), nil
	}
	return table.Num(formats.TimeToSASDate(t)), nil
}

// dateExtract applies f to the civil date of a SAS date argument.
func dateExtract(args []table.Value, name string, f func(time.Time) float64) (table.Value, error) {
	if len(args) != 1 {
		return table.MissingNum(), fmt.Errorf("%s expects 1 argument, got %d", name, len(args))
	}
	if args[0].IsMissing() {
		return table.MissingNum(), nil
	}
	return table.Num(f(formats.SASDateToTime(args[0].Num))), nil
}

func yearFn(args []table.Value) (table.Value, error) {
	return dateExtract(args, "year", func(t time.Time) float64 { return float64(t.Year()) })
}
func monthFn(args []table.Value) (table.Value, error) {
	return dateExtract(args, "month", func(t time.Time) float64 { return float64(int(t.Month())) })
}
func dayFn(args []table.Value) (table.Value, error) {
	return dateExtract(args, "day", func(t time.Time) float64 { return float64(t.Day()) })
}
func qtrFn(args []table.Value) (table.Value, error) {
	return dateExtract(args, "qtr", func(t time.Time) float64 { return float64((int(t.Month())-1)/3 + 1) })
}

// weekday — SAS weekday number: Sunday=1 .. Saturday=7.
func weekdayFn(args []table.Value) (table.Value, error) {
	return dateExtract(args, "weekday", func(t time.Time) float64 { return float64(int(t.Weekday()) + 1) })
}

// datepart — the date (day number) part of a SAS datetime.
func datepartFn(args []table.Value) (table.Value, error) {
	if len(args) != 1 {
		return table.MissingNum(), fmt.Errorf("datepart expects 1 argument, got %d", len(args))
	}
	if args[0].IsMissing() {
		return table.MissingNum(), nil
	}
	return table.Num(math.Floor(args[0].Num / 86400)), nil
}

// timepart — the time (seconds since midnight) part of a SAS datetime.
func timepartFn(args []table.Value) (table.Value, error) {
	if len(args) != 1 {
		return table.MissingNum(), fmt.Errorf("timepart expects 1 argument, got %d", len(args))
	}
	if args[0].IsMissing() {
		return table.MissingNum(), nil
	}
	dt := args[0].Num
	return table.Num(dt - math.Floor(dt/86400)*86400), nil
}

// hms(hour, minute, second) — a SAS time value (seconds since midnight).
func hmsFn(args []table.Value) (table.Value, error) {
	if len(args) != 3 {
		return table.MissingNum(), fmt.Errorf("hms expects 3 arguments, got %d", len(args))
	}
	if args[0].IsMissing() || args[1].IsMissing() || args[2].IsMissing() {
		return table.MissingNum(), nil
	}
	return table.Num(args[0].Num*3600 + args[1].Num*60 + args[2].Num), nil
}

// dhms(date, hour, minute, second) — a SAS datetime value.
func dhmsFn(args []table.Value) (table.Value, error) {
	if len(args) != 4 {
		return table.MissingNum(), fmt.Errorf("dhms expects 4 arguments, got %d", len(args))
	}
	for _, a := range args {
		if a.IsMissing() {
			return table.MissingNum(), nil
		}
	}
	return table.Num(args[0].Num*86400 + args[1].Num*3600 + args[2].Num*60 + args[3].Num), nil
}

// intervalSpec is a parsed SAS interval name (e.g. "MONTH2.2", "WEEK", "DTDAY",
// "HOUR3"). SAS interval syntax is <name><multiplier>.<shift>, optionally with a
// "DT" prefix that reinterprets a date interval against datetime values.
//
// The model is a uniform grid: every interval partitions a one-dimensional grid
// (months, days, or fixed-size seconds) into consecutive runs of `period` grid
// units, with the run boundaries shifted by `offset` units. intck counts the run
// boundaries crossed; intnx jumps `n` runs and aligns within the target run.
type intervalSpec struct {
	grid    string  // "month", "day", or "sec"
	secUnit float64 // grid unit size in seconds (sec grid only): hour=3600, minute=60, second=1
	period  int     // grid units per interval = base * multiplier
	offset  int     // boundary offset in grid units = baseOffset + (shift-1)
	dt      bool    // a date interval (month/day grid) applied to datetime values
}

// parseInterval parses a SAS interval name into an intervalSpec.
func parseInterval(s string) (intervalSpec, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	shift := 1
	if i := strings.IndexByte(s, '.'); i >= 0 {
		if v, err := strconv.Atoi(strings.TrimSpace(s[i+1:])); err == nil && v >= 1 {
			shift = v
		}
		s = s[:i]
	}
	// Trailing digits are the multiplier (e.g. MONTH2, WEEK3).
	mult := 1
	j := len(s)
	for j > 0 && s[j-1] >= '0' && s[j-1] <= '9' {
		j--
	}
	if j < len(s) {
		if v, err := strconv.Atoi(s[j:]); err == nil && v >= 1 {
			mult = v
		}
		s = s[:j]
	}
	dt := false
	if strings.HasPrefix(s, "dt") && len(s) > 2 {
		dt = true
		s = s[2:]
	}

	var sp intervalSpec
	base, baseOffset := 1, 0
	switch s {
	case "day", "days":
		sp.grid = "day"
	case "week", "weeks":
		// SAS weeks start Sunday; day 0 (1960-01-01) is a Friday, so Sundays are
		// the days d with (d-2) divisible by 7. The shift index counts days, so
		// WEEK.2 starts weeks on Monday.
		sp.grid, base, baseOffset = "day", 7, 2
	case "month", "months":
		sp.grid = "month"
	case "qtr", "quarter", "qtrs", "quarters":
		sp.grid, base = "month", 3
	case "semiyear", "semiyears":
		sp.grid, base = "month", 6
	case "year", "years":
		sp.grid, base = "month", 12
	case "hour", "hours":
		sp.grid, sp.secUnit = "sec", 3600
	case "minute", "minutes":
		sp.grid, sp.secUnit = "sec", 60
	case "second", "seconds":
		sp.grid, sp.secUnit = "sec", 1
	default:
		return sp, fmt.Errorf("unsupported interval %q", s)
	}
	// The DT prefix only changes scaling for calendar (month/day) intervals;
	// sub-day intervals already operate on the seconds of a datetime directly.
	sp.dt = dt && sp.grid != "sec"
	sp.period = base * mult
	sp.offset = baseOffset + (shift - 1)
	return sp, nil
}

// toGrid converts a SAS value (date, datetime, or time) to this interval's grid
// domain: a day number for month/day grids, or seconds for the sec grid. A DT
// calendar interval receives datetime seconds, which become day numbers.
func (sp intervalSpec) toGrid(value float64) float64 {
	if sp.dt {
		return value / 86400
	}
	return value
}

// fromGrid converts a grid-domain result back to the SAS value domain.
func (sp intervalSpec) fromGrid(g float64) float64 {
	if sp.dt {
		return g * 86400
	}
	return g
}

// gridIndex maps a grid-domain value to its integer grid-unit index.
func (sp intervalSpec) gridIndex(g float64) int {
	switch sp.grid {
	case "month":
		t := formats.SASDateToTime(math.Floor(g))
		return t.Year()*12 + int(t.Month()) - 1
	case "day":
		return int(math.Floor(g))
	default: // sec
		return int(math.Floor(g / sp.secUnit))
	}
}

// gridBoundary maps a grid-unit index back to the grid-domain value at its start.
func (sp intervalSpec) gridBoundary(i int) float64 {
	switch sp.grid {
	case "month":
		y := i / 12
		m := i % 12
		if m < 0 {
			m += 12
			y--
		}
		return formats.TimeToSASDate(time.Date(y, time.Month(m+1), 1, 0, 0, 0, 0, time.UTC))
	case "day":
		return float64(i)
	default: // sec
		return float64(i) * sp.secUnit
	}
}

// intervalIndex is the ordinal of the interval (run of `period` grid units)
// containing grid-domain value g.
func (sp intervalSpec) intervalIndex(g float64) int {
	return int(math.Floor(float64(sp.gridIndex(g)-sp.offset) / float64(sp.period)))
}

// intck(interval, from, to) — the number of interval boundaries between two
// values. Intervals: day, week, month, qtr, semiyear, year, hour, minute,
// second; an optional multiplier (MONTH2), shift (WEEK.2), and DT prefix
// (DTMONTH, for datetime values).
func intckFn(args []table.Value) (table.Value, error) {
	if len(args) != 3 {
		return table.MissingNum(), fmt.Errorf("intck expects 3 arguments, got %d", len(args))
	}
	if args[1].IsMissing() || args[2].IsMissing() {
		return table.MissingNum(), nil
	}
	sp, err := parseInterval(args[0].Str)
	if err != nil {
		return table.MissingNum(), fmt.Errorf("intck: %v", err)
	}
	from := sp.intervalIndex(sp.toGrid(args[1].Num))
	to := sp.intervalIndex(sp.toGrid(args[2].Num))
	return table.Num(float64(to - from)), nil
}

// intnx(interval, start, increment[, alignment]) — advances start by increment
// intervals and aligns the result within the resulting interval. Alignment:
// b(eginning, default), m(iddle), e(nd), s(ame).
func intnxFn(args []table.Value) (table.Value, error) {
	if len(args) < 3 || len(args) > 4 {
		return table.MissingNum(), fmt.Errorf("intnx expects 3 or 4 arguments, got %d", len(args))
	}
	if args[1].IsMissing() || args[2].IsMissing() {
		return table.MissingNum(), nil
	}
	sp, err := parseInterval(args[0].Str)
	if err != nil {
		return table.MissingNum(), fmt.Errorf("intnx: %v", err)
	}
	n := int(args[2].Num)
	align := "b"
	if len(args) == 4 && strings.TrimSpace(args[3].Str) != "" {
		align = strings.ToLower(strings.TrimSpace(args[3].Str))[:1]
	}

	startG := sp.toGrid(args[1].Num)
	gi := sp.intervalIndex(startG)
	beginGrid := (gi+n)*sp.period + sp.offset
	begin := sp.gridBoundary(beginGrid)
	end := sp.gridBoundary(beginGrid+sp.period) - 1 // last grid unit of the run

	var result float64
	switch align {
	case "e":
		result = end
	case "m":
		result = math.Floor((begin + end) / 2)
	case "s":
		// Preserve the offset of start within its own run.
		within := startG - sp.gridBoundary(gi*sp.period+sp.offset)
		result = begin + within
	default: // "b"
		result = begin
	}
	return table.Num(sp.fromGrid(result)), nil
}

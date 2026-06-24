package runtime

import (
	"fmt"
	"math"
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

// weekIndex maps a SAS day number to its (Sunday-aligned) week ordinal. SAS day 0
// (1960-01-01) is a Friday, so the Sunday on/before it is day -5.
func weekIndex(day float64) int { return int(math.Floor((day + 5) / 7)) }

// addMonths advances (year, month[1..12]) by n months with correct flooring for
// negative n, returning the new year and 1..12 month.
func addMonths(y, m, n int) (int, int) {
	base := y*12 + (m - 1) + n
	ny := base / 12
	nm := base % 12
	if nm < 0 {
		nm += 12
		ny--
	}
	return ny, nm + 1
}

// intck(interval, from, to) — the number of interval boundaries between two
// dates. Supported intervals: day, week, month, qtr/quarter, year.
func intckFn(args []table.Value) (table.Value, error) {
	if len(args) != 3 {
		return table.MissingNum(), fmt.Errorf("intck expects 3 arguments, got %d", len(args))
	}
	if args[1].IsMissing() || args[2].IsMissing() {
		return table.MissingNum(), nil
	}
	interval := strings.ToLower(strings.TrimSpace(args[0].Str))
	from, to := args[1].Num, args[2].Num
	ft := formats.SASDateToTime(from)
	tt := formats.SASDateToTime(to)
	switch interval {
	case "day", "days":
		return table.Num(math.Floor(to) - math.Floor(from)), nil
	case "week", "weeks":
		return table.Num(float64(weekIndex(to) - weekIndex(from))), nil
	case "month", "months":
		return table.Num(float64((tt.Year()-ft.Year())*12 + (int(tt.Month()) - int(ft.Month())))), nil
	case "qtr", "quarter":
		fq := (int(ft.Month()) - 1) / 3
		tq := (int(tt.Month()) - 1) / 3
		return table.Num(float64((tt.Year()-ft.Year())*4 + (tq - fq))), nil
	case "year", "years":
		return table.Num(float64(tt.Year() - ft.Year())), nil
	default:
		return table.MissingNum(), fmt.Errorf("intck: unsupported interval %q", interval)
	}
}

// intnx(interval, start, increment[, alignment]) — advances start by increment
// intervals and aligns the result within the resulting interval. Alignment:
// b(eginning, default), m(iddle), e(nd), s(ame). Intervals: day, week, month,
// qtr/quarter, year.
func intnxFn(args []table.Value) (table.Value, error) {
	if len(args) < 3 || len(args) > 4 {
		return table.MissingNum(), fmt.Errorf("intnx expects 3 or 4 arguments, got %d", len(args))
	}
	if args[1].IsMissing() || args[2].IsMissing() {
		return table.MissingNum(), nil
	}
	interval := strings.ToLower(strings.TrimSpace(args[0].Str))
	start := args[1].Num
	n := int(args[2].Num)
	align := "b"
	if len(args) == 4 && strings.TrimSpace(args[3].Str) != "" {
		align = strings.ToLower(strings.TrimSpace(args[3].Str))[:1]
	}

	// Compute the target interval as [begin, end] day numbers plus the start
	// interval's beginning (for "same" alignment), then apply alignment uniformly.
	var begin, end, beginOfStart float64
	st := formats.SASDateToTime(start)
	switch interval {
	case "day", "days":
		begin = math.Floor(start) + float64(n)
		end = begin
		beginOfStart = math.Floor(start)
	case "week", "weeks":
		curSunday := float64(weekIndex(start)*7 - 5)
		tgtSunday := float64((weekIndex(start)+n)*7 - 5)
		begin, end, beginOfStart = tgtSunday, tgtSunday+6, curSunday
	case "month", "months":
		ny, nm := addMonths(st.Year(), int(st.Month()), n)
		begin = formats.TimeToSASDate(time.Date(ny, time.Month(nm), 1, 0, 0, 0, 0, time.UTC))
		end = formats.TimeToSASDate(time.Date(ny, time.Month(nm)+1, 0, 0, 0, 0, 0, time.UTC))
		beginOfStart = formats.TimeToSASDate(time.Date(st.Year(), st.Month(), 1, 0, 0, 0, 0, time.UTC))
	case "qtr", "quarter":
		fq := (int(st.Month()) - 1) / 3
		ny, nm := addMonths(st.Year(), fq*3+1, n*3)
		begin = formats.TimeToSASDate(time.Date(ny, time.Month(nm), 1, 0, 0, 0, 0, time.UTC))
		end = formats.TimeToSASDate(time.Date(ny, time.Month(nm)+3, 0, 0, 0, 0, 0, time.UTC))
		beginOfStart = formats.TimeToSASDate(time.Date(st.Year(), time.Month(fq*3+1), 1, 0, 0, 0, 0, time.UTC))
	case "year", "years":
		ty := st.Year() + n
		begin = formats.TimeToSASDate(time.Date(ty, 1, 1, 0, 0, 0, 0, time.UTC))
		end = formats.TimeToSASDate(time.Date(ty, 12, 31, 0, 0, 0, 0, time.UTC))
		beginOfStart = formats.TimeToSASDate(time.Date(st.Year(), 1, 1, 0, 0, 0, 0, time.UTC))
	default:
		return table.MissingNum(), fmt.Errorf("intnx: unsupported interval %q", interval)
	}

	switch align {
	case "e":
		return table.Num(end), nil
	case "m":
		return table.Num(math.Floor((begin + end) / 2)), nil
	case "s":
		return table.Num(begin + (math.Floor(start) - beginOfStart)), nil
	default: // "b"
		return table.Num(begin), nil
	}
}

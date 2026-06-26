package runtime

import (
	"testing"

	"github.com/solifugus/ass/table"
)

func nums(vs ...float64) []table.Value {
	out := make([]table.Value, len(vs))
	for i, v := range vs {
		out[i] = table.Num(v)
	}
	return out
}

// wantNum runs fn(args) and asserts a non-missing numeric result equal to want.
func wantNum(t *testing.T, name string, got table.Value, err error, want float64) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", name, err)
	}
	if got.IsMissing() {
		t.Fatalf("%s: got missing, want %v", name, want)
	}
	if got.Num != want {
		t.Errorf("%s: got %v, want %v", name, got.Num, want)
	}
}

func TestDateConstructors(t *testing.T) {
	// 01JAN1960 is SAS day 0; 01JAN2020 is day 21915.
	v, err := mdyFn(nums(1, 1, 1960))
	wantNum(t, "mdy epoch", v, err, 0)
	v, err = mdyFn(nums(1, 1, 2020))
	wantNum(t, "mdy 2020", v, err, 21915)

	// Invalid dates -> missing.
	if v, _ := mdyFn(nums(13, 1, 2020)); !v.IsMissing() {
		t.Errorf("mdy(13,1,2020) should be missing, got %v", v.Num)
	}
	if v, _ := mdyFn(nums(2, 30, 2020)); !v.IsMissing() {
		t.Errorf("mdy(2,30,2020) should be missing, got %v", v.Num)
	}
	// Missing argument propagates.
	if v, _ := mdyFn([]table.Value{table.MissingNum(), table.Num(1), table.Num(2020)}); !v.IsMissing() {
		t.Errorf("mdy with missing arg should be missing")
	}
}

func TestDateExtractors(t *testing.T) {
	d := 21915.0 // 01JAN2020 (a Wednesday)
	v, err := yearFn(nums(d))
	wantNum(t, "year", v, err, 2020)
	v, err = monthFn(nums(d))
	wantNum(t, "month", v, err, 1)
	v, err = dayFn(nums(d))
	wantNum(t, "day", v, err, 1)
	v, err = qtrFn(nums(d))
	wantNum(t, "qtr", v, err, 1)
	v, err = weekdayFn(nums(d))
	wantNum(t, "weekday 2020-01-01 (Wed)", v, err, 4)
	// 01JAN1960 was a Friday -> SAS weekday 6.
	v, err = weekdayFn(nums(0))
	wantNum(t, "weekday epoch (Fri)", v, err, 6)
}

func TestTimeParts(t *testing.T) {
	v, err := hmsFn(nums(1, 2, 3))
	wantNum(t, "hms", v, err, 3723)
	// dhms(21915, 1, 2, 3) = 21915*86400 + 3723.
	dt, err := dhmsFn(nums(21915, 1, 2, 3))
	wantNum(t, "dhms", dt, err, 21915*86400+3723)
	v, err = datepartFn(nums(dt.Num))
	wantNum(t, "datepart", v, err, 21915)
	v, err = timepartFn(nums(dt.Num))
	wantNum(t, "timepart", v, err, 3723)
}

func TestIntck(t *testing.T) {
	d2000, d2020 := 14610.0, 21915.0 // 01JAN2000, 01JAN2020
	v, err := intckFn([]table.Value{table.Char("year"), table.Num(d2000), table.Num(d2020)})
	wantNum(t, "intck year", v, err, 20)

	jan15, mar20 := 21929.0, 21994.0 // 15JAN2020, 20MAR2020
	v, err = intckFn([]table.Value{table.Char("month"), table.Num(jan15), table.Num(mar20)})
	wantNum(t, "intck month", v, err, 2)

	v, err = intckFn([]table.Value{table.Char("day"), table.Num(100), table.Num(110)})
	wantNum(t, "intck day", v, err, 10)

	v, err = intckFn([]table.Value{table.Char("week"), table.Num(0), table.Num(7)})
	wantNum(t, "intck week", v, err, 1)

	v, err = intckFn([]table.Value{table.Char("qtr"), table.Num(jan15), table.Num(mar20)})
	wantNum(t, "intck qtr (same quarter)", v, err, 0)
}

func TestIntnx(t *testing.T) {
	jan15 := 21929.0 // 15JAN2020
	v, err := intnxFn([]table.Value{table.Char("month"), table.Num(jan15), table.Num(1)})
	wantNum(t, "intnx month begin", v, err, 21946) // 01FEB2020
	v, err = intnxFn([]table.Value{table.Char("month"), table.Num(jan15), table.Num(1), table.Char("e")})
	wantNum(t, "intnx month end", v, err, 21974) // 29FEB2020 (leap)
	v, err = intnxFn([]table.Value{table.Char("month"), table.Num(jan15), table.Num(1), table.Char("s")})
	wantNum(t, "intnx month same", v, err, 21960) // 15FEB2020

	v, err = intnxFn([]table.Value{table.Char("year"), table.Num(jan15), table.Num(1)})
	wantNum(t, "intnx year begin", v, err, 22281) // 01JAN2021

	feb15 := 21960.0 // 15FEB2020 (Q1)
	v, err = intnxFn([]table.Value{table.Char("qtr"), table.Num(feb15), table.Num(1)})
	wantNum(t, "intnx qtr begin", v, err, 22006) // 01APR2020

	v, err = intnxFn([]table.Value{table.Char("day"), table.Num(100), table.Num(5)})
	wantNum(t, "intnx day", v, err, 105)
}

func TestIntervalAdvanced(t *testing.T) {
	const (
		jan15 = 21929.0 // 15JAN2020 (a date)
		day   = 86400.0
		wed   = 5.0     // 1960-01-06 is a Wednesday (day 0 = Friday)
		t1430 = 52200.0 // 14:30:00 in seconds since midnight
	)

	// Multiplier: MONTH2 partitions into bimonthly Jan/Mar/May/... runs.
	v, err := intnxFn([]table.Value{table.Char("month2"), table.Num(jan15), table.Num(1)})
	wantNum(t, "intnx month2 begin", v, err, 21975) // 01MAR2020
	v, err = intckFn([]table.Value{table.Char("month2"), table.Num(jan15), table.Num(22036)})
	wantNum(t, "intck month2", v, err, 2) // 15JAN -> 01MAY = 2 bimonthly boundaries

	// Shift: WEEK.2 starts weeks on Monday (vs Sunday for plain WEEK).
	v, err = intnxFn([]table.Value{table.Char("week"), table.Num(wed), table.Num(0)})
	wantNum(t, "intnx week begin (Sunday)", v, err, 2) // 1960-01-03 Sun
	v, err = intnxFn([]table.Value{table.Char("week.2"), table.Num(wed), table.Num(0)})
	wantNum(t, "intnx week.2 begin (Monday)", v, err, 3) // 1960-01-04 Mon

	// SEMIYEAR: half-year runs Jan/Jul.
	v, err = intnxFn([]table.Value{table.Char("semiyear"), table.Num(jan15), table.Num(1)})
	wantNum(t, "intnx semiyear begin", v, err, 22097) // 01JUL2020

	// DT-prefixed calendar intervals operate on datetime (seconds) values.
	v, err = intckFn([]table.Value{table.Char("dtday"), table.Num(jan15 * day), table.Num((jan15 + 3) * day)})
	wantNum(t, "intck dtday", v, err, 3)
	v, err = intnxFn([]table.Value{table.Char("dtmonth"), table.Num(jan15 * day), table.Num(1)})
	wantNum(t, "intnx dtmonth begin", v, err, 21946*day) // 01FEB2020 00:00:00

	// Sub-day intervals: hour / minute / second on seconds values.
	v, err = intnxFn([]table.Value{table.Char("hour"), table.Num(t1430), table.Num(2)})
	wantNum(t, "intnx hour begin", v, err, 57600) // 16:00:00
	v, err = intnxFn([]table.Value{table.Char("hour"), table.Num(t1430), table.Num(0), table.Char("e")})
	wantNum(t, "intnx hour end", v, err, 53999) // 14:59:59
	v, err = intckFn([]table.Value{table.Char("hour"), table.Num(t1430), table.Num(t1430 + 2*3600)})
	wantNum(t, "intck hour", v, err, 2)
	v, err = intnxFn([]table.Value{table.Char("minute"), table.Num(90), table.Num(1)})
	wantNum(t, "intnx minute begin", v, err, 120) // 00:02:00

	// Multiplier on hours: HOUR3 partitions the day into 3-hour runs from midnight.
	v, err = intnxFn([]table.Value{table.Char("hour3"), table.Num(t1430), table.Num(1)})
	wantNum(t, "intnx hour3 begin", v, err, 54000) // 14:30 in [12:00,15:00) -> next run 15:00
}

func TestTodayIsSane(t *testing.T) {
	v, err := todayFn(nil)
	if err != nil || v.IsMissing() {
		t.Fatalf("today() returned err=%v missing=%v", err, v.IsMissing())
	}
	y, _ := yearFn(nums(v.Num))
	if y.Num < 2025 {
		t.Errorf("year(today()) = %v, expected a current year", y.Num)
	}
}

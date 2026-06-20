package formats

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/solifugus/ass/table"
)

// sasEpoch is SAS day 0: 1960-01-01. A SAS date value is the integer count of
// days since this date.
var sasEpoch = time.Date(1960, 1, 1, 0, 0, 0, 0, time.UTC)

var monthAbbr = []string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}
var monthName = []string{"January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December"}

// SASDateToTime converts a SAS day number to a civil date (UTC).
func SASDateToTime(day float64) time.Time { return sasEpoch.AddDate(0, 0, int(day)) }

// ParseDateLiteral parses a SAS date constant body like "01JAN2020" (the text
// inside the quotes of a `'...'d` literal) into a SAS day number.
func ParseDateLiteral(s string) (float64, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) < 7 {
		return 0, false
	}
	// dd MMM yy|yyyy — the day is 1-2 digits, month is 3 letters.
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i+3 > len(s) {
		return 0, false
	}
	day, _ := strconv.Atoi(s[:i])
	mon := s[i : i+3]
	yearStr := s[i+3:]
	month := 0
	for m, a := range monthAbbr {
		if a == mon {
			month = m + 1
			break
		}
	}
	if month == 0 {
		return 0, false
	}
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		return 0, false
	}
	if year < 100 { // 2-digit year: SAS yearcutoff-style (1920..2019)
		if year < 20 {
			year += 2000
		} else {
			year += 1900
		}
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return float64(int(t.Sub(sasEpoch).Hours()) / 24), true
}

// ParseTimeLiteral parses a SAS time constant body like "14:30:00" or "9:15"
// (the text inside the quotes of a `'...'t` literal) into a SAS time value: the
// number of seconds since midnight. Fractional seconds are accepted.
func ParseTimeLiteral(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || m < 0 || m > 59 {
		return 0, false
	}
	sec := 0.0
	if len(parts) == 3 {
		var err3 error
		sec, err3 = strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		if err3 != nil || sec < 0 || sec >= 60 {
			return 0, false
		}
	}
	return float64(h)*3600 + float64(m)*60 + sec, true
}

// ParseDatetimeLiteral parses a SAS datetime constant body like
// "01JAN2020:14:30:00" (the text inside the quotes of a `'...'dt` literal) into a
// SAS datetime value: the number of seconds since 1960-01-01 00:00:00. The date
// and time are separated by the first colon; the time part is optional.
func ParseDatetimeLiteral(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	i := strings.IndexByte(s, ':')
	datePart, timePart := s, ""
	if i >= 0 {
		datePart, timePart = s[:i], s[i+1:]
	}
	day, ok := ParseDateLiteral(datePart)
	if !ok {
		return 0, false
	}
	secs := 0.0
	if strings.TrimSpace(timePart) != "" {
		t, ok := ParseTimeLiteral(timePart)
		if !ok {
			return 0, false
		}
		secs = t
	}
	return day*86400 + secs, true
}

// ParseInput converts an input field to a Value using an informat specification
// (e.g. "comma8.", "dollar10.2", "date9.", "mmddyy10.", "$20."). The informat's
// name determines the conversion: `$` reads characters (truncated to the width);
// `comma`/`dollar` strip grouping/currency symbols; `date`/`mmddyy`/`ddmmyy`/
// `yymmdd` read dates to SAS day numbers; anything else parses a plain number.
// An empty/"." field yields the appropriate missing value.
func ParseInput(field, informat string) table.Value {
	name, width, _, _ := parseSpec(informat)
	field = strings.TrimSpace(field)

	if name == "$" {
		if width > 0 && len(field) > width {
			field = field[:width]
		}
		return table.Char(field)
	}
	if field == "" || field == "." {
		return table.MissingNum()
	}

	switch name {
	case "comma", "dollar", "comman", "dollarn":
		clean := strings.Map(func(r rune) rune {
			if r == ',' || r == '$' || r == ' ' {
				return -1
			}
			return r
		}, field)
		f, err := strconv.ParseFloat(clean, 64)
		if err != nil {
			return table.MissingNum()
		}
		return table.Num(f)
	case "date":
		if d, ok := ParseDateLiteral(field); ok {
			return table.Num(d)
		}
		return table.MissingNum()
	case "mmddyy", "ddmmyy", "yymmdd":
		if d, ok := parseNumericDate(field, name); ok {
			return table.Num(d)
		}
		return table.MissingNum()
	case "time", "hhmmss":
		if t, ok := ParseTimeLiteral(field); ok {
			return table.Num(t)
		}
		return table.MissingNum()
	case "datetime":
		if dt, ok := ParseDatetimeLiteral(field); ok {
			return table.Num(dt)
		}
		return table.MissingNum()
	default: // plain numeric w.d
		f, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return table.MissingNum()
		}
		return table.Num(f)
	}
}

// parseNumericDate reads a date written with `/`, `-`, or `.` separators (or as
// packed digits) in the given order (mmddyy/ddmmyy/yymmdd) and returns its SAS
// day number.
func parseNumericDate(s, order string) (float64, bool) {
	var parts []string
	if strings.ContainsAny(s, "/-.") {
		parts = strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == '-' || r == '.' })
	} else { // packed digits: 2/2/2 or 2/2/4
		switch len(s) {
		case 6:
			parts = []string{s[0:2], s[2:4], s[4:6]}
		case 8:
			if order == "yymmdd" {
				parts = []string{s[0:4], s[4:6], s[6:8]}
			} else {
				parts = []string{s[0:2], s[2:4], s[4:8]}
			}
		default:
			return 0, false
		}
	}
	if len(parts) != 3 {
		return 0, false
	}
	var mo, dy, yr int
	var err error
	switch order {
	case "mmddyy":
		mo, dy, yr, err = atoi3(parts[0], parts[1], parts[2])
	case "ddmmyy":
		dy, mo, yr, err = atoi3(parts[0], parts[1], parts[2])
	case "yymmdd":
		yr, mo, dy, err = atoi3(parts[0], parts[1], parts[2])
	}
	if err != nil || mo < 1 || mo > 12 || dy < 1 || dy > 31 {
		return 0, false
	}
	if yr < 100 {
		if yr < 20 {
			yr += 2000
		} else {
			yr += 1900
		}
	}
	t := time.Date(yr, time.Month(mo), dy, 0, 0, 0, 0, time.UTC)
	return float64(int(t.Sub(sasEpoch).Hours()) / 24), true
}

func atoi3(a, b, c string) (x, y, z int, err error) {
	if x, err = strconv.Atoi(a); err != nil {
		return
	}
	if y, err = strconv.Atoi(b); err != nil {
		return
	}
	z, err = strconv.Atoi(c)
	return
}

// Apply renders a value using a SAS format specification (e.g. "8.2",
// "dollar10.2", "comma12.0", "percent8.1", "$8."). An empty or unrecognized
// format falls back to the value's default display. Numeric missing renders as
// ".".
func Apply(v table.Value, format string) string {
	if format == "" {
		return v.Display()
	}
	name, width, dec, hasDec := parseSpec(format)

	if v.Kind == table.Character || name == "$" {
		s := v.Str
		if width > 0 && len(s) > width {
			s = s[:width]
		}
		return s
	}
	if v.IsMissing() {
		return "."
	}

	switch name {
	case "dollar":
		n, sign := v.Num, ""
		if n < 0 {
			n, sign = -n, "-"
		}
		return sign + "$" + group(n, decOr(dec, hasDec, 2))
	case "comma":
		return group(v.Num, decOr(dec, hasDec, 0))
	case "percent":
		return fixed(v.Num*100, decOr(dec, hasDec, 0)) + "%"
	case "date":
		t := SASDateToTime(v.Num)
		if width >= 9 {
			return fmt.Sprintf("%02d%s%04d", t.Day(), monthAbbr[t.Month()-1], t.Year())
		}
		return fmt.Sprintf("%02d%s%02d", t.Day(), monthAbbr[t.Month()-1], t.Year()%100)
	case "mmddyy":
		t := SASDateToTime(v.Num)
		if width >= 10 {
			return fmt.Sprintf("%02d/%02d/%04d", int(t.Month()), t.Day(), t.Year())
		}
		return fmt.Sprintf("%02d/%02d/%02d", int(t.Month()), t.Day(), t.Year()%100)
	case "worddate":
		t := SASDateToTime(v.Num)
		return fmt.Sprintf("%s %d, %d", monthName[t.Month()-1], t.Day(), t.Year())
	case "time", "hhmmss":
		h, m, s := splitTime(v.Num)
		if width >= 5 && width < 8 { // e.g. time5. -> HH:MM
			return fmt.Sprintf("%02d:%02d", h, m)
		}
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	case "datetime":
		days := int(v.Num / 86400)
		rem := v.Num - float64(days)*86400
		if rem < 0 { // negative datetimes: borrow a day
			days--
			rem += 86400
		}
		t := SASDateToTime(float64(days))
		h, m, s := splitTime(rem)
		return fmt.Sprintf("%02d%s%04d:%02d:%02d:%02d", t.Day(), monthAbbr[t.Month()-1], t.Year(), h, m, s)
	default: // "", "best", "f" → fixed-point if decimals given, else default
		if hasDec {
			return fixed(v.Num, dec)
		}
		return v.Display()
	}
}

// splitTime converts a SAS time value (seconds since midnight) into whole
// hours, minutes, and seconds.
func splitTime(secs float64) (h, m, s int) {
	total := int(secs)
	return total / 3600, (total % 3600) / 60, total % 60
}

// decOr returns dec if a decimal count was specified, otherwise def.
func decOr(dec int, hasDec bool, def int) int {
	if hasDec {
		return dec
	}
	return def
}

// fixed formats n with exactly d decimal places.
func fixed(n float64, d int) string {
	return strconv.FormatFloat(n, 'f', d, 64)
}

// group formats n with thousands separators and d decimal places.
func group(n float64, d int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatFloat(n, 'f', d, 64)
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	// Insert commas every three digits from the right.
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String() + frac
	if neg {
		out = "-" + out
	}
	return out
}

// parseSpec splits a format specification into its name, width, and optional
// decimal count. Examples: "8.2" -> ("",8,2,true); "dollar10.2" ->
// ("dollar",10,2,true); "$8." -> ("$",8,0,false); "comma12." -> ("comma",12,0,false).
func parseSpec(f string) (name string, width, dec int, hasDec bool) {
	i := 0
	if i < len(f) && f[i] == '$' {
		name = "$"
		i++
	}
	for i < len(f) && isLetter(f[i]) {
		i++
	}
	if name == "" {
		name = strings.ToLower(f[:i])
	} else {
		name += strings.ToLower(f[1:i])
	}
	// width digits
	j := i
	for j < len(f) && isDigit(f[j]) {
		j++
	}
	width, _ = strconv.Atoi(f[i:j])
	if j < len(f) && f[j] == '.' {
		k := j + 1
		for k < len(f) && isDigit(f[k]) {
			k++
		}
		if k > j+1 {
			dec, _ = strconv.Atoi(f[j+1 : k])
			hasDec = true
		}
	}
	return name, width, dec, hasDec
}

func isLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
func isDigit(b byte) bool  { return b >= '0' && b <= '9' }

// Package flatfile holds the low-level delimited-text helpers shared by the
// DATA step's INFILE/FILE statements and PROC IMPORT/EXPORT. Keeping the
// reading, field-splitting, DSD quoting, and writing in one place lets the
// runtime and proc packages agree on flat-file semantics without a dependency
// cycle (runtime imports proc, so the shared code lives below both).
package flatfile

import (
	"fmt"
	"os"
	"strings"
)

// ReadLines reads a flat file into one record per line, applying FIRSTOBS=/OBS=
// line bounds (1-based; 0 = unset). CRLF is normalized to LF and a single
// trailing newline is dropped.
func ReadLines(path string, firstobs, obs int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", path, err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil, nil
	}
	lines := strings.Split(text, "\n")
	first := firstobs
	if first < 1 {
		first = 1
	}
	if first > len(lines) {
		return nil, nil
	}
	last := len(lines)
	if obs > 0 && obs < last {
		last = obs
	}
	return lines[first-1 : last], nil
}

// WriteLines writes each line followed by a newline to path (mode 0o644),
// overwriting any existing file.
func WriteLines(path string, lines []string) error {
	var b strings.Builder
	for _, ln := range lines {
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("%q: %w", path, err)
	}
	return nil
}

// AppendLines appends each line (followed by a newline) to path, creating the
// file if it does not exist — the FILE statement's MOD option.
func AppendLines(path string, lines []string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("%q: %w", path, err)
	}
	defer f.Close()
	var b strings.Builder
	for _, ln := range lines {
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	if _, err := f.WriteString(b.String()); err != nil {
		return fmt.Errorf("%q: %w", path, err)
	}
	return nil
}

// SplitDelim breaks one record into fields on the separator. With dsd=false a
// run of consecutive delimiters collapses to one (SAS DLM-without-DSD behavior).
// With dsd=true it uses CSV-style parsing: quoted fields may contain the
// delimiter, "" is an escaped quote, and consecutive delimiters yield empty
// (missing) fields.
func SplitDelim(line string, sep rune, dsd bool) []string {
	if dsd {
		return splitDSD(line, sep)
	}
	return strings.FieldsFunc(line, func(r rune) bool { return r == sep })
}

// splitDSD parses one delimited line with DSD semantics.
func splitDSD(line string, sep rune) []string {
	var fields []string
	var b strings.Builder
	inQuotes := false
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case inQuotes:
			if r == '"' {
				if i+1 < len(runes) && runes[i+1] == '"' {
					b.WriteRune('"')
					i++
				} else {
					inQuotes = false
				}
			} else {
				b.WriteRune(r)
			}
		case r == '"':
			inQuotes = true
		case r == sep:
			fields = append(fields, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	fields = append(fields, b.String())
	return fields
}

// Quote wraps s in double quotes (doubling embedded quotes) when it contains the
// separator, a quote, or a newline — the DSD/CSV output convention. Otherwise s
// is returned unchanged.
func Quote(s, sep string) string {
	if strings.Contains(s, sep) || strings.Contains(s, "\"") || strings.Contains(s, "\n") {
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}
	return s
}

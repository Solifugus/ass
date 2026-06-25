package formats

import (
	"fmt"
	"html"
	"strings"
)

// TitleText renders the active TITLE lines as plain text (each on its own line,
// followed by a blank separator), shown above procedure output in batch/REPL.
// Returns "" when there are no titles, so callers emit nothing.
func TitleText(titles []string) string {
	if len(titles) == 0 {
		return ""
	}
	var b strings.Builder
	for _, t := range titles {
		b.WriteString(t)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.String()
}

// TitleHTML renders the active TITLE lines as a centered heading block for rich
// (notebook) output: the first line largest, the rest progressively smaller, all
// HTML-escaped. Returns "" when there are no titles.
func TitleHTML(titles []string) string {
	if len(titles) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div style="text-align:left;margin:8px 0 2px;color:inherit;font-family:ui-sans-serif,-apple-system,Segoe UI,Roboto,sans-serif">`)
	for i, t := range titles {
		size, weight := "13px", "600"
		switch i {
		case 0:
			size, weight = "16px", "700"
		case 1:
			size = "14px"
		}
		fmt.Fprintf(&b, `<div style="font-size:%s;font-weight:%s">%s</div>`, size, weight, html.EscapeString(t))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// Shared CSS for rich (notebook) HTML output. These styles are used by the PROC
// table renderers (package proc) and the PROC PROOF panel (package runtime), so
// every ASS table in a notebook looks consistent. Colors are grayscale rgba
// overlays over the theme background and text inherits the theme foreground, so
// the tables render correctly on both light and dark notebook themes without
// detecting which is in use.
const (
	HTMLTableStyle   = "border-collapse:collapse;font-family:ui-sans-serif,-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;font-size:13px;line-height:1.45;margin:6px 0;color:inherit"
	HTMLCaptionStyle = "caption-side:top;text-align:left;font-weight:600;padding:0 2px 7px;font-size:13px"
	HTMLThStyle      = "padding:4px 11px;background:rgba(127,127,127,.16);border-bottom:2px solid rgba(127,127,127,.5);white-space:nowrap;"
	HTMLTdStyle      = "padding:3px 11px;border-bottom:1px solid rgba(127,127,127,.16);white-space:nowrap;"
	HTMLZebraStyle   = "background:rgba(127,127,127,.06)"
	HTMLNumStyle     = "text-align:right;font-variant-numeric:tabular-nums"
	HTMLTextStyle    = "text-align:left"
)

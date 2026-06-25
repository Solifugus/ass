package formats

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

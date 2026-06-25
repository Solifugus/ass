package proc

import (
	"strings"
	"testing"

	"github.com/solifugus/ass/table"
)

func TestRenderHTMLListing(t *testing.T) {
	got := renderHTMLListing(samplePeople(), printOptions{}, "WORK.PEOPLE")
	for _, want := range []string{
		"<table",
		"<caption", "WORK.PEOPLE", "2 rows &times; 2 cols", // caption with dims
		">Obs</th>", ">name</th>", ">age</th>",
		">John</td>",
		"text-align:right", // numeric column right-aligned
		"</table>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("HTML listing missing %q; got:\n%s", want, got)
		}
	}
}

func TestRenderHTMLListingNoobs(t *testing.T) {
	got := renderHTMLListing(samplePeople(), printOptions{noobs: true}, "")
	if strings.Contains(got, ">Obs</th>") {
		t.Errorf("noobs HTML should omit the Obs column; got:\n%s", got)
	}
	if strings.Contains(got, "<caption") {
		t.Errorf("empty caption should produce no <caption>; got:\n%s", got)
	}
}

// TestRenderHTMLListingSingularDims checks the row/col pluralization in the
// caption (1 row, 1 col → no trailing "s").
func TestRenderHTMLListingSingularDims(t *testing.T) {
	ds := table.NewDataset("", "t")
	ds.AddColumn(table.Column{Name: "x", Kind: table.Numeric})
	ds.AppendRow(table.Row{"x": table.Num(1)})
	got := renderHTMLListing(ds, printOptions{}, "WORK.T")
	if !strings.Contains(got, "1 row &times; 1 col") {
		t.Errorf("expected singular dims; got:\n%s", got)
	}
}

// TestRenderHTMLListingEscapes confirms cell values are HTML-escaped (no markup
// injection from data).
func TestRenderHTMLListingEscapes(t *testing.T) {
	ds := table.NewDataset("", "t")
	ds.AddColumn(table.Column{Name: "v", Kind: table.Character})
	ds.AppendRow(table.Row{"v": table.Char("a<b>&c")})
	got := renderHTMLListing(ds, printOptions{}, "")
	if strings.Contains(got, "<b>") {
		t.Errorf("value markup not escaped; got:\n%s", got)
	}
	if !strings.Contains(got, "a&lt;b&gt;&amp;c") {
		t.Errorf("expected escaped value; got:\n%s", got)
	}
}

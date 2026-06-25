package proc

import (
	"strings"
	"testing"

	"github.com/solifugus/ass/table"
)

func TestRenderHTMLListing(t *testing.T) {
	got := renderHTMLListing(samplePeople(), printOptions{})
	for _, want := range []string{
		"<table",
		"<th>Obs</th>", "<th>name</th>", "<th>age</th>",
		"<td>John</td>",
		`<td style="text-align:right">25</td>`, // numeric column is right-aligned
		"</table>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("HTML listing missing %q; got:\n%s", want, got)
		}
	}
}

func TestRenderHTMLListingNoobs(t *testing.T) {
	got := renderHTMLListing(samplePeople(), printOptions{noobs: true})
	if strings.Contains(got, "<th>Obs</th>") {
		t.Errorf("noobs HTML should omit the Obs column; got:\n%s", got)
	}
}

// TestRenderHTMLListingEscapes confirms cell values are HTML-escaped (no markup
// injection from data).
func TestRenderHTMLListingEscapes(t *testing.T) {
	ds := table.NewDataset("", "t")
	ds.AddColumn(table.Column{Name: "v", Kind: table.Character})
	ds.AppendRow(table.Row{"v": table.Char("a<b>&c")})
	got := renderHTMLListing(ds, printOptions{})
	if strings.Contains(got, "<b>") {
		t.Errorf("value markup not escaped; got:\n%s", got)
	}
	if !strings.Contains(got, "a&lt;b&gt;&amp;c") {
		t.Errorf("expected escaped value; got:\n%s", got)
	}
}

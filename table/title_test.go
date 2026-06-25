package table

import (
	"strings"
	"testing"
)

func TestLibraryTitles(t *testing.T) {
	l := NewLibrary()
	if len(l.TitleLines()) != 0 {
		t.Fatal("new library should have no titles")
	}

	l.SetTitle(1, "Report")
	l.SetTitle(2, "Subtitle")
	l.SetTitle(4, "Footer line") // gaps are allowed; only set levels show
	if got := strings.Join(l.TitleLines(), "|"); got != "Report|Subtitle|Footer line" {
		t.Errorf("TitleLines = %q", got)
	}

	// title3; (empty) clears level 3 and every higher one, leaving 1 and 2.
	l.SetTitle(3, "")
	if got := strings.Join(l.TitleLines(), "|"); got != "Report|Subtitle" {
		t.Errorf("after clear-from-3 TitleLines = %q, want Report|Subtitle", got)
	}

	// A bare title; (level 1, empty) clears everything.
	l.SetTitle(1, "")
	if len(l.TitleLines()) != 0 {
		t.Errorf("after bare clear, titles = %v, want none", l.TitleLines())
	}
}

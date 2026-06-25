package parser

import (
	"testing"

	"github.com/solifugus/ass/ast"
)

func TestParseTitle(t *testing.T) {
	prog := New(`title "Main";
title2 'Sub';
title3;
proc print data=x; run;`).ParseProgram()

	var titles []*ast.TitleStatement
	for _, s := range prog.Steps {
		if ts, ok := s.(*ast.TitleStatement); ok {
			titles = append(titles, ts)
		}
	}
	if len(titles) != 3 {
		t.Fatalf("got %d title statements, want 3", len(titles))
	}
	if titles[0].Level != 1 || titles[0].Text != "Main" {
		t.Errorf("title1 = %+v, want {1 Main}", titles[0])
	}
	if titles[1].Level != 2 || titles[1].Text != "Sub" {
		t.Errorf("title2 = %+v, want {2 Sub}", titles[1])
	}
	if titles[2].Level != 3 || titles[2].Text != "" {
		t.Errorf("title3 = %+v, want {3 <empty>} (clear)", titles[2])
	}
}

// TestTitleNotConfusedWithVar ensures a variable or step named like "titles"
// isn't misparsed — only title/title1..title10 are title statements.
func TestParseTitleLevels(t *testing.T) {
	for _, c := range []struct {
		ident string
		want  int
	}{
		{"title", 1}, {"TITLE", 1}, {"title1", 1}, {"title9", 9}, {"title10", 10},
		{"title0", 0}, {"title11", 0}, {"titles", 0}, {"titled", 0}, {"foo", 0},
	} {
		if got := titleLevel(c.ident); got != c.want {
			t.Errorf("titleLevel(%q) = %d, want %d", c.ident, got, c.want)
		}
	}
}

package lexer

import "testing"

// collect drains the lexer into a slice of tokens up to and including EOF.
func collect(input string) []Token {
	l := New(input)
	var toks []Token
	for {
		t := l.NextToken()
		toks = append(toks, t)
		if t.Type == EOF {
			return toks
		}
	}
}

func TestNextTokenBasic(t *testing.T) {
	input := `data x; total = qty * price + 1; run;`
	want := []struct {
		typ TokenType
		lit string
	}{
		{DATA, "data"},
		{IDENT, "x"},
		{SEMICOLON, ";"},
		{IDENT, "total"},
		{EQ, "="},
		{IDENT, "qty"},
		{STAR, "*"},
		{IDENT, "price"},
		{PLUS, "+"},
		{NUMBER, "1"},
		{SEMICOLON, ";"},
		{RUN, "run"},
		{SEMICOLON, ";"},
		{EOF, ""},
	}
	got := collect(input)
	if len(got) != len(want) {
		t.Fatalf("token count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Type != w.typ || got[i].Literal != w.lit {
			t.Errorf("token %d = {%s %q}, want {%s %q}", i, got[i].Type, got[i].Literal, w.typ, w.lit)
		}
	}
}

func TestOperators(t *testing.T) {
	input := `= ^= ~= <> < <= > >= ** || & | ^ % , . ( )`
	want := []TokenType{
		EQ, NE, NE, NE, LT, LE, GT, GE, POW, CONCAT, AMP, PIPE, NOT, PERCENT, COMMA, DOT, LPAREN, RPAREN, EOF,
	}
	got := collect(input)
	if len(got) != len(want) {
		t.Fatalf("token count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Type != w {
			t.Errorf("token %d = %s (%q), want %s", i, got[i].Type, got[i].Literal, w)
		}
	}
}

func TestNumbers(t *testing.T) {
	cases := map[string]string{
		"123":    "123",
		"4.5":    "4.5",
		".5":     ".5",
		"1e3":    "1e3",
		"2.5E-2": "2.5E-2",
	}
	for in, lit := range cases {
		got := collect(in)
		if got[0].Type != NUMBER || got[0].Literal != lit {
			t.Errorf("%q => {%s %q}, want {NUMBER %q}", in, got[0].Type, got[0].Literal, lit)
		}
	}
}

func TestStrings(t *testing.T) {
	cases := map[string]string{
		`'hello'`:      "hello",
		`"world"`:      "world",
		`'it''s'`:      "it's",
		`"say ""hi"""`: `say "hi"`,
	}
	for in, val := range cases {
		got := collect(in)
		if got[0].Type != STRING || got[0].Literal != val {
			t.Errorf("%q => {%s %q}, want {STRING %q}", in, got[0].Type, got[0].Literal, val)
		}
	}
}

func TestKeywordsCaseInsensitive(t *testing.T) {
	got := collect("DATA Proc RUN quit Cards")
	want := []TokenType{DATA, PROC, RUN, QUIT, DATALINES, EOF}
	for i, w := range want {
		if got[i].Type != w {
			t.Errorf("token %d = %s, want %s", i, got[i].Type, w)
		}
	}
}

func TestPositions(t *testing.T) {
	// "x = 1;\ny = 2;" — check line/col of the second line's first ident.
	got := collect("x = 1;\ny = 2;")
	var y Token
	for _, tk := range got {
		if tk.Literal == "y" {
			y = tk
			break
		}
	}
	if y.Line != 2 || y.Col != 1 {
		t.Errorf("y at line %d col %d, want line 2 col 1", y.Line, y.Col)
	}
}

func TestIllegal(t *testing.T) {
	got := collect("@")
	if got[0].Type != ILLEGAL || got[0].Literal != "@" {
		t.Errorf("got {%s %q}, want {ILLEGAL \"@\"}", got[0].Type, got[0].Literal)
	}
}

func TestBlockComment(t *testing.T) {
	got := collect("x /* this is\na comment */ = 1;")
	want := []TokenType{IDENT, EQ, NUMBER, SEMICOLON, EOF}
	if len(got) != len(want) {
		t.Fatalf("token count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Type != w {
			t.Errorf("token %d = %s, want %s", i, got[i].Type, w)
		}
	}
}

func TestStatementComment(t *testing.T) {
	// A leading '*' at statement start is a comment to the next ';'.
	got := collect("* a throwaway comment ; x = 1;")
	want := []TokenType{IDENT, EQ, NUMBER, SEMICOLON, EOF}
	if len(got) != len(want) {
		t.Fatalf("token count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Type != w {
			t.Errorf("token %d = %s (%q), want %s", i, got[i].Type, got[i].Literal, w)
		}
	}
}

func TestStarIsMultiplyNotComment(t *testing.T) {
	// A '*' that is NOT at statement start must remain multiplication.
	got := collect("y = a * b;")
	want := []TokenType{IDENT, EQ, IDENT, STAR, IDENT, SEMICOLON, EOF}
	if len(got) != len(want) {
		t.Fatalf("token count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Type != w {
			t.Errorf("token %d = %s, want %s", i, got[i].Type, w)
		}
	}
}

func TestDatalines(t *testing.T) {
	input := "data people;\n  input name $ age;\n  datalines;\nJohn 25\nMary 30\n;\nrun;\n"
	got := collect(input)
	want := []struct {
		typ TokenType
		lit string
	}{
		{DATA, "data"},
		{IDENT, "people"},
		{SEMICOLON, ";"},
		{IDENT, "input"},
		{IDENT, "name"},
		{DOLLAR, "$"},
		{IDENT, "age"},
		{SEMICOLON, ";"},
		{DATALINES, "datalines"},
		{SEMICOLON, ";"},
		{DATALINES_DATA, "John 25\nMary 30"},
		{SEMICOLON, ";"}, // the terminator line
		{RUN, "run"},
		{SEMICOLON, ";"},
		{EOF, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("token count = %d, want %d\n got: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Type != w.typ || got[i].Literal != w.lit {
			t.Errorf("token %d = {%s %q}, want {%s %q}", i, got[i].Type, got[i].Literal, w.typ, w.lit)
		}
	}
}

func TestDollarInInput(t *testing.T) {
	got := collect("input name $ age;")
	want := []TokenType{IDENT, IDENT, DOLLAR, IDENT, SEMICOLON, EOF}
	if len(got) != len(want) {
		t.Fatalf("token count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Type != w {
			t.Errorf("token %d = %s, want %s", i, got[i].Type, w)
		}
	}
}

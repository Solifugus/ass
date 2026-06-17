package lexer

import "strings"

// Lexer turns SAS source text into a stream of Tokens via repeated NextToken
// calls. It tracks 1-based line and column positions. Comment handling and the
// inline-datalines mode are layered on in later steps.
type Lexer struct {
	src  []rune // source as runes (SAS source is small; rune slice keeps positions simple)
	pos  int    // index of the current rune in src
	line int    // 1-based line of the current rune
	col  int    // 1-based column of the current rune

	// lastType is the type of the previously emitted token. It is used to
	// detect statement-start, where a leading '*' begins a statement comment.
	// It is seeded to SEMICOLON so the very start of the program counts as a
	// statement start.
	lastType TokenType

	// pendingData is set after a `datalines;`/`cards;` statement so the next
	// scan reads the raw inline data block instead of normal tokens.
	pendingData bool
}

// New creates a Lexer over the given source.
func New(input string) *Lexer {
	return &Lexer{src: []rune(input), pos: 0, line: 1, col: 1, lastType: SEMICOLON}
}

// Slice returns the source text between two rune indices (e.g. Token.Pos/End),
// used to recover an exact source span such as a raw PROC SQL body. Out-of-range
// indices are clamped.
func (l *Lexer) Slice(start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(l.src) {
		end = len(l.src)
	}
	if start >= end {
		return ""
	}
	return string(l.src[start:end])
}

// cur returns the current rune, or 0 at end of input.
func (l *Lexer) cur() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

// peek returns the rune n positions ahead of the current one, or 0 past the end.
func (l *Lexer) peek(n int) rune {
	i := l.pos + n
	if i >= len(l.src) {
		return 0
	}
	return l.src[i]
}

// advance consumes the current rune, updating line/column counters.
func (l *Lexer) advance() rune {
	r := l.cur()
	l.pos++
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func (l *Lexer) skipWhitespace() {
	for {
		switch l.cur() {
		case ' ', '\t', '\r', '\n':
			l.advance()
		default:
			return
		}
	}
}

// atStmtStart reports whether the lexer is positioned where a new statement may
// begin, i.e. immediately after a ';' (or at the very start of the program).
// SAS treats a leading '*' there as a statement comment rather than a multiply.
func (l *Lexer) atStmtStart() bool { return l.lastType == SEMICOLON }

// skipTrivia skips whitespace and both SAS comment forms, repeatedly, until the
// current rune is the start of a real token.
//   - Block comment:     / * ... * /  (anywhere)
//   - Statement comment:  * ... ;       (only at statement start)
func (l *Lexer) skipTrivia() {
	for {
		l.skipWhitespace()
		switch {
		case l.cur() == '/' && l.peek(1) == '*':
			l.advance()
			l.advance()
			for !(l.cur() == '*' && l.peek(1) == '/') && l.cur() != 0 {
				l.advance()
			}
			if l.cur() != 0 { // consume the closing */
				l.advance()
				l.advance()
			}
		case l.cur() == '*' && l.atStmtStart():
			for l.cur() != ';' && l.cur() != 0 {
				l.advance()
			}
			if l.cur() == ';' {
				l.advance() // consume the terminating ; (statement stays "started")
			}
		default:
			return
		}
	}
}

// NextToken scans and returns the next token, remembering its type so the next
// call can detect statement-start. At end of input it returns EOF. After a
// `datalines;` statement, the following call returns the raw data block as a
// single DATALINES_DATA token.
func (l *Lexer) NextToken() Token {
	prev := l.lastType
	tok := l.scan()
	// Entering datalines mode: the ';' that closes a `datalines` statement.
	if tok.Type == SEMICOLON && prev == DATALINES {
		l.pendingData = true
	}
	l.lastType = tok.Type
	return tok
}

// scan produces the next token without bookkeeping (see NextToken). It stamps
// each token's source span (Pos/End as rune indices) via a deferred assignment
// so the many return sites below need not set them individually.
func (l *Lexer) scan() (tok Token) {
	if l.pendingData {
		l.pendingData = false
		return l.readDatalines()
	}

	l.skipTrivia()

	start := l.pos
	defer func() {
		tok.Pos = start
		tok.End = l.pos
	}()

	line, col := l.line, l.col
	r := l.cur()

	switch {
	case r == 0:
		return Token{Type: EOF, Literal: "", Line: line, Col: col}
	case isIdentStart(r):
		lit := l.readIdentifier()
		return Token{Type: LookupIdent(lit), Literal: lit, Line: line, Col: col}
	case isDigit(r):
		return Token{Type: NUMBER, Literal: l.readNumber(), Line: line, Col: col}
	case r == '.' && isDigit(l.peek(1)):
		return Token{Type: NUMBER, Literal: l.readNumber(), Line: line, Col: col}
	case r == '\'' || r == '"':
		return Token{Type: STRING, Literal: l.readString(r), Line: line, Col: col}
	}

	// Operators and punctuation (single- and multi-rune).
	switch r {
	case '=':
		l.advance()
		return Token{Type: EQ, Literal: "=", Line: line, Col: col}
	case '+':
		l.advance()
		return Token{Type: PLUS, Literal: "+", Line: line, Col: col}
	case '-':
		l.advance()
		return Token{Type: MINUS, Literal: "-", Line: line, Col: col}
	case '*':
		l.advance()
		if l.cur() == '*' {
			l.advance()
			return Token{Type: POW, Literal: "**", Line: line, Col: col}
		}
		return Token{Type: STAR, Literal: "*", Line: line, Col: col}
	case '/':
		l.advance()
		return Token{Type: SLASH, Literal: "/", Line: line, Col: col}
	case '<':
		l.advance()
		switch l.cur() {
		case '=':
			l.advance()
			return Token{Type: LE, Literal: "<=", Line: line, Col: col}
		case '>':
			l.advance()
			return Token{Type: NE, Literal: "<>", Line: line, Col: col}
		}
		return Token{Type: LT, Literal: "<", Line: line, Col: col}
	case '>':
		l.advance()
		if l.cur() == '=' {
			l.advance()
			return Token{Type: GE, Literal: ">=", Line: line, Col: col}
		}
		return Token{Type: GT, Literal: ">", Line: line, Col: col}
	case '^', '~':
		l.advance()
		if l.cur() == '=' {
			l.advance()
			return Token{Type: NE, Literal: "^=", Line: line, Col: col}
		}
		return Token{Type: NOT, Literal: string(r), Line: line, Col: col}
	case '|':
		l.advance()
		if l.cur() == '|' {
			l.advance()
			return Token{Type: CONCAT, Literal: "||", Line: line, Col: col}
		}
		return Token{Type: PIPE, Literal: "|", Line: line, Col: col}
	case '&':
		l.advance()
		return Token{Type: AMP, Literal: "&", Line: line, Col: col}
	case '%':
		l.advance()
		return Token{Type: PERCENT, Literal: "%", Line: line, Col: col}
	case ';':
		l.advance()
		return Token{Type: SEMICOLON, Literal: ";", Line: line, Col: col}
	case '(':
		l.advance()
		return Token{Type: LPAREN, Literal: "(", Line: line, Col: col}
	case ')':
		l.advance()
		return Token{Type: RPAREN, Literal: ")", Line: line, Col: col}
	case '[':
		l.advance()
		return Token{Type: LBRACKET, Literal: "[", Line: line, Col: col}
	case ']':
		l.advance()
		return Token{Type: RBRACKET, Literal: "]", Line: line, Col: col}
	case '{':
		l.advance()
		return Token{Type: LBRACE, Literal: "{", Line: line, Col: col}
	case '}':
		l.advance()
		return Token{Type: RBRACE, Literal: "}", Line: line, Col: col}
	case ',':
		l.advance()
		return Token{Type: COMMA, Literal: ",", Line: line, Col: col}
	case '.':
		l.advance()
		return Token{Type: DOT, Literal: ".", Line: line, Col: col}
	case '$':
		l.advance()
		return Token{Type: DOLLAR, Literal: "$", Line: line, Col: col}
	}

	// Unrecognized rune.
	l.advance()
	return Token{Type: ILLEGAL, Literal: string(r), Line: line, Col: col}
}

// readIdentifier consumes an identifier (letter/underscore start, then
// letters, digits, underscores).
func (l *Lexer) readIdentifier() string {
	start := l.pos
	for isIdentPart(l.cur()) {
		l.advance()
	}
	return string(l.src[start:l.pos])
}

// readNumber consumes a numeric literal: digits, an optional fractional part,
// and an optional exponent (e/E with optional sign).
func (l *Lexer) readNumber() string {
	start := l.pos
	for isDigit(l.cur()) {
		l.advance()
	}
	if l.cur() == '.' {
		l.advance()
		for isDigit(l.cur()) {
			l.advance()
		}
	}
	if l.cur() == 'e' || l.cur() == 'E' {
		l.advance()
		if l.cur() == '+' || l.cur() == '-' {
			l.advance()
		}
		for isDigit(l.cur()) {
			l.advance()
		}
	}
	return string(l.src[start:l.pos])
}

// readString consumes a quoted string and returns its unquoted value. A doubled
// quote inside the string ('' or "") is an escaped single quote character.
func (l *Lexer) readString(quote rune) string {
	l.advance() // opening quote
	var out []rune
	for {
		c := l.cur()
		if c == 0 {
			break // unterminated string: return what we have
		}
		if c == quote {
			if l.peek(1) == quote { // doubled quote -> literal quote
				l.advance()
				l.advance()
				out = append(out, quote)
				continue
			}
			l.advance() // closing quote
			break
		}
		out = append(out, c)
		l.advance()
	}
	return string(out)
}

// lineFrom returns the text of the line starting at rune index i, up to (but
// not including) the next newline or end of input.
func (l *Lexer) lineFrom(i int) string {
	j := i
	for j < len(l.src) && l.src[j] != '\n' {
		j++
	}
	return string(l.src[i:j])
}

// readDatalines reads the raw inline data block following a `datalines;`
// statement. Lines are captured verbatim until a terminator line (a line whose
// trimmed content is ";" or ";;;;"), which is left in place so the normal
// scanner emits its closing SEMICOLON. The block is returned as one token with
// lines joined by "\n".
func (l *Lexer) readDatalines() Token {
	// Advance past the remainder of the `datalines;` line, including its newline,
	// so we start at the first data line.
	for l.cur() != '\n' && l.cur() != 0 {
		l.advance()
	}
	if l.cur() == '\n' {
		l.advance()
	}

	line, col := l.line, l.col
	var lines []string
	for l.cur() != 0 {
		text := l.lineFrom(l.pos)
		if t := strings.TrimSpace(text); t == ";" || t == ";;;;" {
			break // terminator: leave it for the normal scanner
		}
		for l.cur() != '\n' && l.cur() != 0 { // consume this data line
			l.advance()
		}
		lines = append(lines, text)
		if l.cur() == '\n' {
			l.advance()
		}
	}
	return Token{Type: DATALINES_DATA, Literal: strings.Join(lines, "\n"), Line: line, Col: col}
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isIdentStart(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isIdentPart(r rune) bool { return isIdentStart(r) || isDigit(r) }

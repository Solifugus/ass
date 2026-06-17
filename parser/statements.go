package parser

import (
	"strconv"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/lexer"
)

// identIs reports whether the current token is an IDENT whose lowercased
// literal equals kw (SAS statement keywords arrive as IDENT tokens).
func (p *Parser) identIs(kw string) bool {
	return p.curIs(lexer.IDENT) && strings.ToLower(p.cur.Literal) == kw
}

// peekIdentIs is identIs for the lookahead token.
func (p *Parser) peekIdentIs(kw string) bool {
	return p.peek.Type == lexer.IDENT && strings.ToLower(p.peek.Literal) == kw
}

// parseDataStatement parses one statement in a DATA step body.
func (p *Parser) parseDataStatement() ast.Statement {
	switch {
	case p.curIs(lexer.DATALINES):
		return p.parseDatalines()
	case p.curIs(lexer.SEMICOLON):
		p.next() // empty statement
		return nil
	case p.identIs("set"):
		return p.parseSet()
	case p.identIs("merge"):
		return p.parseMerge()
	case p.identIs("input"):
		return p.parseInput()
	case p.identIs("if"):
		return p.parseIf()
	case p.identIs("where"):
		return p.parseWhere()
	case p.identIs("do"):
		return p.parseDo()
	case p.identIs("output"):
		return p.parseOutput()
	case p.identIs("keep"):
		return p.parseNameListStmt("keep")
	case p.identIs("drop"):
		return p.parseNameListStmt("drop")
	case p.identIs("format"):
		return p.parseFormatStmt()
	case p.identIs("retain"):
		return p.parseRetain()
	case p.identIs("array"):
		return p.parseArray()
	case p.curIs(lexer.IDENT) && (p.peek.Type == lexer.LBRACE || p.peek.Type == lexer.LBRACKET):
		return p.parseArrayElementAssignment()
	case p.curIs(lexer.IDENT) && p.peek.Type == lexer.PLUS:
		return p.parseSum()
	case p.identIs("by"):
		return p.parseBy()
	case p.curIs(lexer.IDENT) && p.peek.Type == lexer.EQ:
		return p.parseAssignment()
	default:
		return p.parseRawStatement()
	}
}

// parseAssignment parses `<name> = <expr>;`.
func (p *Parser) parseAssignment() ast.Statement {
	name := p.cur.Literal
	p.next() // name
	p.next() // '='
	stmt := &ast.AssignmentStatement{Name: name, Value: p.parseExpression(pLOWEST)}
	p.expectSemicolon()
	return stmt
}

// parseFormatStmt parses `format <var-list> <format.> ...;`. The format tokens
// are recovered from raw source (between the keyword and the ';') because a SAS
// format like `dollar10.2` does not survive tokenization cleanly; a token
// containing '.' is a format, otherwise it is a variable name. A format applies
// to all variables listed since the previous format.
func (p *Parser) parseFormatStmt() ast.Statement {
	p.next() // 'format'
	start := p.cur.Pos
	end := start
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		end = p.cur.End
		p.next()
	}
	raw := p.l.Slice(start, end)
	p.expectSemicolon()

	stmt := &ast.FormatStatement{Formats: map[string]string{}}
	var pending []string
	for _, tok := range strings.Fields(raw) {
		if strings.Contains(tok, ".") { // a format spec (only formats contain '.')
			fm := strings.TrimSuffix(tok, ".") // "comma12." -> "comma12"; "8.2" stays
			for _, v := range pending {
				stmt.Formats[strings.ToLower(v)] = fm
			}
			pending = nil
		} else {
			pending = append(pending, tok)
		}
	}
	return stmt
}

// parseRetain parses `retain <var [initial]> ...;`. Identifiers are variable
// names; a literal that follows assigns an initial value to the most recent
// variable.
func (p *Parser) parseRetain() ast.Statement {
	p.next() // 'retain'
	stmt := &ast.RetainStatement{Initials: map[string]ast.Expression{}}
	last := ""
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		if p.curIs(lexer.IDENT) {
			last = p.cur.Literal
			stmt.Vars = append(stmt.Vars, last)
			p.next()
			continue
		}
		expr := p.parsePrefixExpr() // a numeric/string/(-num) initial value
		if last != "" {
			stmt.Initials[strings.ToLower(last)] = expr
		}
	}
	p.expectSemicolon()
	return stmt
}

// parseArray parses `array name{n|*} elem1 elem2 ...;`. Element lists may use
// `x1-x3` numeric-suffix ranges, which are expanded.
func (p *Parser) parseArray() ast.Statement {
	p.next() // 'array'
	stmt := &ast.ArrayStatement{}
	if p.curIs(lexer.IDENT) {
		stmt.Name = p.cur.Literal
		p.next()
	}
	// Dimension: {n} / [n] / {*} (parens are not used to avoid call ambiguity).
	if p.curIs(lexer.LBRACE) || p.curIs(lexer.LBRACKET) {
		close := lexer.RBRACE
		if p.curIs(lexer.LBRACKET) {
			close = lexer.RBRACKET
		}
		p.next()
		if p.curIs(lexer.NUMBER) {
			n, _ := strconv.Atoi(p.cur.Literal)
			stmt.Size = n
			p.next()
		} else if p.curIs(lexer.STAR) {
			p.next() // '*' => size inferred from element count
		}
		if p.curIs(close) {
			p.next()
		}
	}
	// Element names (until ';'), expanding x1-x3 ranges.
	for p.curIs(lexer.IDENT) {
		name := p.cur.Literal
		p.next()
		if p.curIs(lexer.MINUS) && p.peek.Type == lexer.IDENT {
			p.next() // '-'
			end := p.cur.Literal
			p.next()
			stmt.Elements = append(stmt.Elements, expandRange(name, end)...)
		} else {
			stmt.Elements = append(stmt.Elements, name)
		}
	}
	if stmt.Size == 0 {
		stmt.Size = len(stmt.Elements)
	}
	p.expectSemicolon()
	return stmt
}

// parseArrayElementAssignment parses `name{index} = value;`.
func (p *Parser) parseArrayElementAssignment() ast.Statement {
	name := p.cur.Literal
	p.next() // name
	ref := p.parseArrayRef(name).(*ast.ArrayRef)
	if p.curIs(lexer.EQ) {
		p.next()
	} else {
		p.addError("expected '=' in array assignment at line " + itoa(p.cur.Line))
	}
	stmt := &ast.ArrayElementAssignment{Name: name, Index: ref.Index, Value: p.parseExpression(pLOWEST)}
	p.expectSemicolon()
	return stmt
}

// expandRange expands a `prefixN - prefixM` element range, e.g. x1-x3 ->
// [x1 x2 x3]. If the names do not share a prefix with numeric suffixes, it
// returns the two endpoints unchanged.
func expandRange(start, end string) []string {
	ps, ns := splitSuffix(start)
	pe, ne := splitSuffix(end)
	if ps == "" || ps != pe || ns < 0 || ne < ns {
		return []string{start, end}
	}
	var out []string
	for i := ns; i <= ne; i++ {
		out = append(out, ps+itoa(i))
	}
	return out
}

// splitSuffix splits a name into its non-numeric prefix and trailing integer
// (e.g. "x12" -> "x", 12). Returns ("",-1) if there is no trailing number.
func splitSuffix(name string) (string, int) {
	i := len(name)
	for i > 0 && name[i-1] >= '0' && name[i-1] <= '9' {
		i--
	}
	if i == len(name) {
		return "", -1
	}
	n := 0
	for _, c := range name[i:] {
		n = n*10 + int(c-'0')
	}
	return name[:i], n
}

// parseSum parses the sum statement `<var> + <expr>;`.
func (p *Parser) parseSum() ast.Statement {
	name := p.cur.Literal
	p.next() // var
	p.next() // '+'
	stmt := &ast.SumStatement{Var: name, Expr: p.parseExpression(pLOWEST)}
	p.expectSemicolon()
	return stmt
}

// parseWhere parses `where <cond>;`.
func (p *Parser) parseWhere() ast.Statement {
	p.next() // 'where'
	stmt := &ast.WhereStatement{Condition: p.parseExpression(pLOWEST)}
	p.expectSemicolon()
	return stmt
}

// parseSet parses `set <dataset...>;`.
func (p *Parser) parseSet() ast.Statement {
	p.next() // 'set'
	stmt := &ast.SetStatement{Datasets: p.parseDatasetNames()}
	p.expectSemicolon()
	return stmt
}

// parseMerge parses `merge ds1[(in=a)] ds2[(in=b)] ...;`.
func (p *Parser) parseMerge() ast.Statement {
	p.next() // 'merge'
	stmt := &ast.MergeStatement{}
	for p.curIs(lexer.IDENT) {
		ref := ast.DatasetRef{Name: p.cur.Literal}
		p.next()
		if p.curIs(lexer.LPAREN) {
			p.next() // '('
			// Dataset options: only in= is interpreted; others are skipped.
			for !p.curIs(lexer.RPAREN) && !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) {
				opt := strings.ToLower(p.cur.Literal)
				p.next()
				if p.curIs(lexer.EQ) {
					p.next()
					val := p.cur.Literal
					p.next()
					if opt == "in" {
						ref.In = val
					}
				}
			}
			if p.curIs(lexer.RPAREN) {
				p.next()
			}
		}
		stmt.Refs = append(stmt.Refs, ref)
	}
	p.expectSemicolon()
	return stmt
}

// parseInput parses `input <var [$]>...;`.
func (p *Parser) parseInput() ast.Statement {
	p.next() // 'input'
	stmt := &ast.InputStatement{}
	for p.curIs(lexer.IDENT) {
		v := ast.InputVar{Name: p.cur.Literal}
		p.next()
		if p.curIs(lexer.DOLLAR) {
			v.Char = true
			p.next()
		}
		stmt.Vars = append(stmt.Vars, v)
	}
	p.expectSemicolon()
	return stmt
}

// parseIf parses both `if <cond> then <stmt>; [else <stmt>;]` and the bare
// subsetting `if <cond>;`.
func (p *Parser) parseIf() ast.Statement {
	p.next() // 'if'
	cond := p.parseExpression(pLOWEST)
	if p.identIs("then") {
		p.next()
		stmt := &ast.IfStatement{Condition: cond, Consequence: p.parseDataStatement()}
		if p.identIs("else") {
			p.next()
			stmt.Alternative = p.parseDataStatement()
		}
		return stmt
	}
	// Subsetting if.
	stmt := &ast.SubsettingIf{Condition: cond}
	p.expectSemicolon()
	return stmt
}

// parseDo parses the DO ... END forms (simple, iterative, while, until).
func (p *Parser) parseDo() ast.Statement {
	p.next() // 'do'
	stmt := &ast.DoStatement{}
	switch {
	case p.curIs(lexer.SEMICOLON):
		stmt.Kind = ast.DoSimple
		p.expectSemicolon()
	case p.identIs("while"):
		stmt.Kind = ast.DoWhile
		p.next()
		stmt.Cond = p.parseParenCond()
		p.expectSemicolon()
	case p.identIs("until"):
		stmt.Kind = ast.DoUntil
		p.next()
		stmt.Cond = p.parseParenCond()
		p.expectSemicolon()
	default: // iterative: do i = from to to [by by];
		stmt.Kind = ast.DoIterative
		if p.curIs(lexer.IDENT) {
			stmt.Var = p.cur.Literal
			p.next()
		}
		if p.curIs(lexer.EQ) {
			p.next()
		}
		stmt.From = p.parseExpression(pLOWEST)
		if p.identIs("to") {
			p.next()
			stmt.To = p.parseExpression(pLOWEST)
		}
		if p.identIs("by") {
			p.next()
			stmt.By = p.parseExpression(pLOWEST)
		}
		p.expectSemicolon()
	}
	stmt.Body = p.parseDoBody()
	return stmt
}

// parseParenCond parses `( <expr> )`.
func (p *Parser) parseParenCond() ast.Expression {
	if p.curIs(lexer.LPAREN) {
		p.next()
	}
	cond := p.parseExpression(pLOWEST)
	if p.curIs(lexer.RPAREN) {
		p.next()
	}
	return cond
}

// parseDoBody collects statements until a matching `end;`.
func (p *Parser) parseDoBody() []ast.Statement {
	var body []ast.Statement
	for !p.identIs("end") && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		if stmt := p.parseDataStatement(); stmt != nil {
			body = append(body, stmt)
		}
	}
	if p.identIs("end") {
		p.next()
		p.expectSemicolon()
	}
	return body
}

// parseOutput parses `output [datasets...];`.
func (p *Parser) parseOutput() ast.Statement {
	p.next() // 'output'
	stmt := &ast.OutputStatement{Datasets: p.parseDatasetNames()}
	p.expectSemicolon()
	return stmt
}

// parseNameListStmt parses `keep`/`drop <vars...>;`.
func (p *Parser) parseNameListStmt(kw string) ast.Statement {
	p.next() // the keyword
	var vars []string
	for p.curIs(lexer.IDENT) {
		vars = append(vars, p.cur.Literal)
		p.next()
	}
	p.expectSemicolon()
	if kw == "keep" {
		return &ast.KeepStatement{Vars: vars}
	}
	return &ast.DropStatement{Vars: vars}
}

// parseBy parses `by [descending] <var> ...;`.
func (p *Parser) parseBy() ast.Statement {
	p.next() // 'by'
	stmt := &ast.ByStatement{}
	desc := false
	for p.curIs(lexer.IDENT) {
		if strings.ToLower(p.cur.Literal) == "descending" {
			desc = true
			p.next()
			continue
		}
		stmt.Vars = append(stmt.Vars, p.cur.Literal)
		stmt.Descending = append(stmt.Descending, desc)
		desc = false
		p.next()
	}
	p.expectSemicolon()
	return stmt
}

// parseDatasetNames reads a run of dataset names, each possibly `lib.name`.
func (p *Parser) parseDatasetNames() []string {
	var names []string
	for p.curIs(lexer.IDENT) {
		name := p.cur.Literal
		p.next()
		if p.curIs(lexer.DOT) {
			p.next()
			if p.curIs(lexer.IDENT) {
				name += "." + p.cur.Literal
				p.next()
			}
		}
		names = append(names, name)
	}
	return names
}

// parseProcStatement parses one statement in a PROC step body. Common report
// statements get dedicated nodes; the rest fall back to RawStatement.
func (p *Parser) parseProcStatement() ast.Statement {
	switch {
	case p.curIs(lexer.SEMICOLON):
		p.next()
		return nil
	case p.identIs("by"):
		return p.parseBy()
	case p.identIs("format"):
		return p.parseFormatStmt()
	case p.identIs("var"):
		return &ast.VarStatement{Vars: p.parseProcNameList()}
	case p.identIs("class"):
		return &ast.ClassStatement{Vars: p.parseProcNameList()}
	case p.identIs("tables") || p.identIs("table"):
		return &ast.TablesStatement{Vars: p.parseProcNameList()}
	default:
		return p.parseRawStatement()
	}
}

// parseProcNameList parses `<keyword> <name name ...>;` and returns the names.
func (p *Parser) parseProcNameList() []string {
	p.next() // the leading keyword
	var names []string
	for p.curIs(lexer.IDENT) {
		names = append(names, p.cur.Literal)
		p.next()
	}
	p.expectSemicolon()
	return names
}

package parser

import (
	"fmt"
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
	case p.identIs("infile"):
		return p.parseInfile()
	case p.identIs("file"):
		return p.parseFile()
	case p.identIs("put"):
		return p.parsePut()
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
	case p.identIs("rename") && p.peek.Type == lexer.IDENT:
		// `rename old=new ...;` statement; `rename = x` would assign the variable
		// named rename (rename is not reserved), but `rename old` (IDENT peek) is
		// the statement form.
		return p.parseRenameStmt()
	case p.identIs("format"):
		return p.parseFormatStmt()
	case p.identIs("label") && p.peek.Type != lexer.EQ:
		// `label x = "..."` is a LABEL statement; `label = "..."` assigns the
		// variable named label (label is not reserved in SAS).
		return p.parseLabelStmt()
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

// parseLabelStmt parses `label <var> = "text" ...;`, associating descriptive
// label text with one or more variables. Each pair is `name = string`; the `=`
// and surrounding spacing are flexible, mirroring SAS.
func (p *Parser) parseLabelStmt() ast.Statement {
	p.next() // 'label'
	stmt := &ast.LabelStatement{Labels: map[string]string{}}
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		if !p.curIs(lexer.IDENT) {
			p.next() // skip stray tokens defensively
			continue
		}
		name := p.cur.Literal
		p.next() // name
		if p.curIs(lexer.EQ) {
			p.next() // '='
		}
		if p.curIs(lexer.STRING) {
			stmt.Labels[strings.ToLower(name)] = p.cur.Literal
			p.next()
		}
	}
	p.expectSemicolon()
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
	stmt := &ast.SetStatement{}
	for p.curIs(lexer.IDENT) {
		stmt.Refs = append(stmt.Refs, p.parseDatasetRef())
	}
	p.expectSemicolon()
	return stmt
}

// parseMerge parses `merge ds1[(opts)] ds2[(opts)] ...;` where opts may include
// in=, keep=, drop=, rename=(...), where=(...).
func (p *Parser) parseMerge() ast.Statement {
	p.next() // 'merge'
	stmt := &ast.MergeStatement{}
	for p.curIs(lexer.IDENT) {
		stmt.Refs = append(stmt.Refs, p.parseDatasetRef())
	}
	p.expectSemicolon()
	return stmt
}

// parseDatasetRef parses a (possibly library-qualified) dataset name followed by
// an optional parenthesized dataset-option list.
func (p *Parser) parseDatasetRef() ast.DatasetRef {
	ref := ast.DatasetRef{Name: p.parseQualifiedName()}
	if p.curIs(lexer.LPAREN) {
		ref.In, ref.Options = p.parseDatasetOptionParen()
	}
	return ref
}

// parseQualifiedName reads `name` or `lib.name`.
func (p *Parser) parseQualifiedName() string {
	name := p.cur.Literal
	p.next()
	if p.curIs(lexer.DOT) {
		p.next()
		if p.curIs(lexer.IDENT) {
			name += "." + p.cur.Literal
			p.next()
		}
	}
	return name
}

// parseDatasetOptionParen parses `(keep=... drop=... rename=(o=n ...) where=(...)
// in=flag)`. It returns the in= flag (if any) and the remaining options (nil if
// none impose a transformation). Assumes cur is the opening LPAREN.
func (p *Parser) parseDatasetOptionParen() (in string, opts *ast.DatasetOptions) {
	p.next() // '('
	opts = &ast.DatasetOptions{}
	for !p.curIs(lexer.RPAREN) && !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) {
		if !p.curIs(lexer.IDENT) {
			p.next()
			continue
		}
		key := strings.ToLower(p.cur.Literal)
		p.next()
		if !p.curIs(lexer.EQ) {
			continue
		}
		p.next() // '='
		switch key {
		case "keep":
			opts.Keep = append(opts.Keep, p.parseOptionVarList()...)
		case "drop":
			opts.Drop = append(opts.Drop, p.parseOptionVarList()...)
		case "rename":
			if opts.Rename == nil {
				opts.Rename = map[string]string{}
			}
			p.parseRenameList(opts.Rename)
		case "where":
			opts.Where = p.parseParenCond()
		case "firstobs":
			if p.curIs(lexer.NUMBER) {
				opts.FirstObs = atoiSafe(p.cur.Literal)
				p.next()
			}
		case "obs":
			if p.curIs(lexer.NUMBER) {
				opts.Obs = atoiSafe(p.cur.Literal)
				p.next()
			}
		case "in":
			if p.curIs(lexer.IDENT) {
				in = p.cur.Literal
				p.next()
			}
		default:
			p.skipOptionValue()
		}
	}
	if p.curIs(lexer.RPAREN) {
		p.next()
	}
	if opts.IsEmpty() {
		opts = nil
	}
	return in, opts
}

// parseOptionVarList collects a space-separated variable list (for keep=/drop=),
// stopping at `)`, `;`, or the start of the next `option=` clause. A numbered
// range `name1-name5` (lexed as IDENT MINUS IDENT) expands to name1..name5.
func (p *Parser) parseOptionVarList() []string {
	var vars []string
	for p.curIs(lexer.IDENT) {
		if p.peek.Type == lexer.EQ { // this ident begins the next option
			break
		}
		lo := p.cur.Literal
		p.next()
		if p.curIs(lexer.MINUS) && p.peek.Type == lexer.IDENT {
			p.next() // consume '-'
			hi := p.cur.Literal
			p.next()
			vars = append(vars, expandVarRange(lo, hi)...)
			continue
		}
		vars = append(vars, lo)
	}
	return vars
}

// expandVarRange expands a numbered variable range `lo`-`hi` (e.g. x1-x5) into
// the list x1,x2,...,x5. Both endpoints must share a common non-numeric prefix
// and end in digits, with hi's number >= lo's; otherwise the two names are
// returned unchanged. The generated suffix is zero-padded to lo's digit width
// (so x01-x03 yields x01,x02,x03).
func expandVarRange(lo, hi string) []string {
	pre1, digs1 := splitTrailingDigits(lo)
	pre2, digs2 := splitTrailingDigits(hi)
	if digs1 == "" || digs2 == "" || !strings.EqualFold(pre1, pre2) {
		return []string{lo, hi}
	}
	n1, err1 := strconv.Atoi(digs1)
	n2, err2 := strconv.Atoi(digs2)
	if err1 != nil || err2 != nil || n2 < n1 {
		return []string{lo, hi}
	}
	width := len(digs1)
	out := make([]string, 0, n2-n1+1)
	for n := n1; n <= n2; n++ {
		out = append(out, fmt.Sprintf("%s%0*d", pre1, width, n))
	}
	return out
}

// splitTrailingDigits splits s into its non-digit prefix and trailing run of
// digits (e.g. "var12" -> "var","12"; "abc" -> "abc","").
func splitTrailingDigits(s string) (prefix, digits string) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	return s[:i], s[i:]
}

// parseRenameList parses `(old=new old2=new2 ...)` into m (keys lowercased).
func (p *Parser) parseRenameList(m map[string]string) {
	if !p.curIs(lexer.LPAREN) {
		return
	}
	p.next() // '('
	for p.curIs(lexer.IDENT) {
		old := p.cur.Literal
		p.next()
		if p.curIs(lexer.EQ) {
			p.next()
			if p.curIs(lexer.IDENT) {
				m[strings.ToLower(old)] = p.cur.Literal
				p.next()
			}
		}
	}
	if p.curIs(lexer.RPAREN) {
		p.next()
	}
}

// skipOptionValue consumes an unrecognized option's value: a parenthesized group
// or a single token.
func (p *Parser) skipOptionValue() {
	if p.curIs(lexer.LPAREN) {
		depth := 0
		for !p.curIs(lexer.EOF) {
			if p.curIs(lexer.LPAREN) {
				depth++
			} else if p.curIs(lexer.RPAREN) {
				depth--
				if depth == 0 {
					p.next()
					return
				}
			}
			p.next()
		}
		return
	}
	if !p.curIs(lexer.RPAREN) && !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) {
		p.next()
	}
}

// parseInfile parses `infile "<path>" <options>;`. Recognized options:
// dlm=/delimiter="<c>", dsd, firstobs=<n>, obs=<n>, missover, truncover. The
// path may be a quoted string (the usual form) or a bare token (a fileref-style
// word, taken literally). Unknown tokens are skipped for forward compatibility.
func (p *Parser) parseInfile() ast.Statement {
	p.next() // 'infile'
	stmt := &ast.InfileStatement{}
	if p.curIs(lexer.STRING) {
		stmt.Path = p.cur.Literal
		p.next()
	} else if p.curIs(lexer.IDENT) {
		stmt.Path = p.cur.Literal
		p.next()
	}
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		if !p.curIs(lexer.IDENT) {
			p.next()
			continue
		}
		key := strings.ToLower(p.cur.Literal)
		switch key {
		case "dlm", "delimiter":
			p.next()
			if p.curIs(lexer.EQ) {
				p.next()
			}
			if p.curIs(lexer.STRING) || p.curIs(lexer.IDENT) {
				stmt.Delimiter = p.cur.Literal
				p.next()
			}
		case "dsd":
			stmt.DSD = true
			p.next()
		case "missover":
			stmt.Missover = true
			p.next()
		case "truncover":
			stmt.Truncover = true
			p.next()
		case "firstobs":
			p.next()
			if p.curIs(lexer.EQ) {
				p.next()
			}
			if p.curIs(lexer.NUMBER) {
				stmt.Firstobs = atoiSafe(p.cur.Literal)
				p.next()
			}
		case "obs":
			p.next()
			if p.curIs(lexer.EQ) {
				p.next()
			}
			if p.curIs(lexer.NUMBER) {
				stmt.Obs = atoiSafe(p.cur.Literal)
				p.next()
			}
		default:
			p.next()
		}
	}
	p.expectSemicolon()
	return stmt
}

// parseFile parses `file "<path>" <options>;`. Recognized options:
// dlm=/delimiter="<c>" and dsd. The path may be a quoted string (usual) or a
// bare token. Unknown tokens are skipped for forward compatibility.
func (p *Parser) parseFile() ast.Statement {
	p.next() // 'file'
	stmt := &ast.FileStatement{}
	if p.curIs(lexer.STRING) || p.curIs(lexer.IDENT) {
		stmt.Path = p.cur.Literal
		p.next()
	}
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		if !p.curIs(lexer.IDENT) {
			p.next()
			continue
		}
		switch strings.ToLower(p.cur.Literal) {
		case "dlm", "delimiter":
			p.next()
			if p.curIs(lexer.EQ) {
				p.next()
			}
			if p.curIs(lexer.STRING) || p.curIs(lexer.IDENT) {
				stmt.Delimiter = p.cur.Literal
				p.next()
			}
		case "dsd":
			stmt.DSD = true
			p.next()
		default:
			p.next()
		}
	}
	p.expectSemicolon()
	return stmt
}

// parsePut parses `put <item>...;`. The item list is recovered from raw source
// (between the keyword and ';') so quoted literals and format specs survive
// intact, then split by parsePutItems. A token containing '.' is a format for
// the preceding variable; a quoted run is a string literal; anything else is a
// variable name. Constructs beyond this (column pointers `@`, `_all_`) are
// deferred.
func (p *Parser) parsePut() ast.Statement {
	p.next() // 'put'
	start := p.cur.Pos
	end := start
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		end = p.cur.End
		p.next()
	}
	raw := p.l.Slice(start, end)
	p.expectSemicolon()

	// A trailing `@@` (hold the output line across iterations) or `@` (hold it
	// within the iteration) at the very end is a line-hold modifier, not an `@n`
	// column pointer (those carry digits and precede an item). It may be spaced
	// (`x @`) or attached (`x@`). No `@n` pointer or format ever ends a PUT, and a
	// trailing quote guards quoted literals, so a final `@` is always a hold.
	stmt := &ast.PutStatement{}
	trimmed := strings.TrimRight(raw, " \t\r\n")
	switch {
	case strings.HasSuffix(trimmed, "@@"):
		stmt.TrailingAt = 2
		raw = strings.TrimSuffix(trimmed, "@@")
	case strings.HasSuffix(trimmed, "@"):
		stmt.TrailingAt = 1
		raw = strings.TrimSuffix(trimmed, "@")
	}
	stmt.Items = parsePutItems(raw)
	return stmt
}

// parsePutItems splits a PUT item list, respecting quoted string literals
// (single or double quotes, with doubled-quote escapes). A bareword containing
// '.' is treated as a format spec attached to the most recent variable item;
// otherwise it is a variable name.
func parsePutItems(raw string) []ast.PutItem {
	var items []ast.PutItem
	var pendAt, pendPlus, pendLine int
	runes := []rune(raw)
	for i := 0; i < len(runes); {
		r := runes[i]
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			i++
			continue
		}
		if r == '"' || r == '\'' {
			quote := r
			i++
			var b strings.Builder
			for i < len(runes) {
				if runes[i] == quote {
					if i+1 < len(runes) && runes[i+1] == quote { // doubled quote escape
						b.WriteRune(quote)
						i += 2
						continue
					}
					i++
					break
				}
				b.WriteRune(runes[i])
				i++
			}
			items = append(items, ast.PutItem{IsLiteral: true, Literal: b.String(), At: pendAt, Plus: pendPlus, Line: pendLine})
			pendAt, pendPlus, pendLine = 0, 0, 0
			continue
		}
		var b strings.Builder
		for i < len(runes) && runes[i] != ' ' && runes[i] != '\t' && runes[i] != '\n' && runes[i] != '\r' {
			b.WriteRune(runes[i])
			i++
		}
		tok := b.String()
		switch {
		case tok == "$": // character marker in column output — kind is known at runtime
			continue
		case strings.EqualFold(tok, "_all_"): // _all_ pseudo-item: every PDV var as name=value
			items = append(items, ast.PutItem{AllVars: true, At: pendAt, Plus: pendPlus, Line: pendLine})
			pendAt, pendPlus, pendLine = 0, 0, 0
			continue
		case tok == "=": // spaced named-output marker: `x =`
			for j := len(items) - 1; j >= 0; j-- {
				if !items[j].IsLiteral {
					items[j].Named = true
					break
				}
			}
			continue
		case len(tok) > 1 && strings.HasSuffix(tok, "=") && !strings.ContainsAny(tok, "."): // attached named output `x=`
			items = append(items, ast.PutItem{Var: strings.TrimSuffix(tok, "="), Named: true, At: pendAt, Plus: pendPlus, Line: pendLine})
			pendAt, pendPlus, pendLine = 0, 0, 0
			continue
		case strings.HasPrefix(tok, "#") && isDigits(tok[1:]): // #n line pointer
			pendLine, _ = strconv.Atoi(tok[1:])
			continue
		case strings.HasPrefix(tok, "@") && isDigits(tok[1:]): // @n absolute column pointer
			pendAt, _ = strconv.Atoi(tok[1:])
			continue
		case strings.HasPrefix(tok, "+") && len(tok) > 1 && isDigits(tok[1:]): // +n relative skip
			pendPlus, _ = strconv.Atoi(tok[1:])
			continue
		case isColRange(tok): // `1-10` column range for the most recent variable
			for j := len(items) - 1; j >= 0; j-- {
				if !items[j].IsLiteral {
					items[j].ColStart, items[j].ColEnd = splitColRange(tok)
					break
				}
			}
			continue
		case isDigits(tok): // single column for the most recent variable
			for j := len(items) - 1; j >= 0; j-- {
				if !items[j].IsLiteral {
					items[j].ColStart, _ = strconv.Atoi(tok)
					break
				}
			}
			continue
		case strings.Contains(tok, "."): // a format spec for the most recent variable
			for j := len(items) - 1; j >= 0; j-- {
				if !items[j].IsLiteral {
					items[j].Format = strings.TrimSuffix(tok, ".")
					break
				}
			}
			continue
		}
		items = append(items, ast.PutItem{Var: tok, At: pendAt, Plus: pendPlus, Line: pendLine})
		pendAt, pendPlus, pendLine = 0, 0, 0
	}
	return items
}

// atoiSafe parses a non-negative integer from a numeric literal, ignoring any
// fractional part; returns 0 on failure.
func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// parseInput parses `input <var [$]>...;`.
func (p *Parser) parseInput() ast.Statement {
	p.next() // 'input'
	start := p.cur.Pos
	end := start
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		end = p.cur.End
		p.next()
	}
	raw := p.l.Slice(start, end)
	p.expectSemicolon()

	// Tokenize on whitespace: a field is a variable name, a bare `$` (character
	// marker), a `:`/`&` list-input modifier (ignored), an informat spec (the
	// only field that contains `.`, optionally `$`-prefixed for character), a
	// `@n`/`+n` column pointer, or a column spec (`1-10` range or a single `5`).
	// Pointers (`@n`/`+n`) bind to the next variable read; ranges/columns and
	// informats bind to the most recent variable.
	stmt := &ast.InputStatement{}
	toks := strings.Fields(raw)

	// A trailing `@@` (hold across iterations) or `@` (hold within the iteration)
	// at the very end is a line-hold modifier, not an `@n` column pointer (those
	// carry digits and precede a variable). Detect and strip it before the field
	// loop. It may be spaced (`x @@`) or attached (`x@@`).
	if n := len(toks); n > 0 {
		last := toks[n-1]
		switch {
		case last == "@@":
			stmt.TrailingAt = 2
			toks = toks[:n-1]
		case last == "@":
			stmt.TrailingAt = 1
			toks = toks[:n-1]
		case strings.HasSuffix(last, "@@"):
			stmt.TrailingAt = 2
			toks[n-1] = strings.TrimSuffix(last, "@@")
		case strings.HasSuffix(last, "@"):
			// No `@n` column pointer ever ends with `@`, so a trailing `@` here is
			// always a line-hold (e.g. the attached form `x@`).
			stmt.TrailingAt = 1
			toks[n-1] = strings.TrimSuffix(last, "@")
		}
	}

	var pendAt, pendPlus, pendLine int
	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		switch {
		case tok == ":" || tok == "&":
			// list-input modifier — informat still applies to the same variable
		case tok == "$":
			if n := len(stmt.Vars); n > 0 {
				stmt.Vars[n-1].Char = true
			}
		case strings.HasPrefix(tok, "#") && isDigits(tok[1:]): // #n line pointer
			if v, err := strconv.Atoi(tok[1:]); err == nil {
				pendLine = v
			}
		case tok == "#": // spaced form `# 2`
			if i+1 < len(toks) && isDigits(toks[i+1]) {
				i++
				if v, err := strconv.Atoi(toks[i]); err == nil {
					pendLine = v
				}
			}
		case strings.HasPrefix(tok, "@"): // @n absolute column pointer
			num := strings.TrimPrefix(tok, "@")
			if num == "" && i+1 < len(toks) { // spaced form `@ 5`
				i++
				num = toks[i]
			}
			if v, err := strconv.Atoi(num); err == nil {
				pendAt = v
			}
		case strings.HasPrefix(tok, "+") && len(tok) > 1 && isDigits(tok[1:]): // +n relative skip
			if v, err := strconv.Atoi(tok[1:]); err == nil {
				pendPlus = v
			}
		case tok == "+": // spaced form `+ 2`
			if i+1 < len(toks) && isDigits(toks[i+1]) {
				i++
				if v, err := strconv.Atoi(toks[i]); err == nil {
					pendPlus = v
				}
			}
		case tok == "-": // spaced range tail: `1 - 10`
			if n := len(stmt.Vars); n > 0 && i+1 < len(toks) && isDigits(toks[i+1]) {
				i++
				if v, err := strconv.Atoi(toks[i]); err == nil {
					stmt.Vars[n-1].ColEnd = v
				}
			}
		case isColRange(tok): // `1-10` column range for the most recent variable
			if n := len(stmt.Vars); n > 0 {
				s, e := splitColRange(tok)
				stmt.Vars[n-1].ColStart = s
				stmt.Vars[n-1].ColEnd = e
			}
		case isDigits(tok): // single column for the most recent variable
			if n := len(stmt.Vars); n > 0 {
				if v, err := strconv.Atoi(tok); err == nil {
					stmt.Vars[n-1].ColStart = v
				}
			}
		case strings.Contains(tok, "."): // an informat for the most recent variable
			if n := len(stmt.Vars); n > 0 {
				inf := tok
				if strings.HasPrefix(inf, "$") {
					stmt.Vars[n-1].Char = true
				}
				stmt.Vars[n-1].Informat = inf
			}
		default: // a new variable name (may carry a trailing `$`)
			name := tok
			char := false
			if strings.HasSuffix(name, "$") {
				name = strings.TrimSuffix(name, "$")
				char = true
			}
			stmt.Vars = append(stmt.Vars, ast.InputVar{Name: name, Char: char, At: pendAt, Plus: pendPlus, Line: pendLine})
			pendAt, pendPlus, pendLine = 0, 0, 0
		}
	}
	return stmt
}

// isDigits reports whether s is non-empty and all ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isColRange reports whether tok is a `start-end` column range (digits, a single
// hyphen, digits).
func isColRange(tok string) bool {
	dash := strings.IndexByte(tok, '-')
	if dash <= 0 || dash == len(tok)-1 {
		return false
	}
	return isDigits(tok[:dash]) && isDigits(tok[dash+1:])
}

// splitColRange parses a validated `start-end` range into its 1-based bounds.
func splitColRange(tok string) (int, int) {
	dash := strings.IndexByte(tok, '-')
	s, _ := strconv.Atoi(tok[:dash])
	e, _ := strconv.Atoi(tok[dash+1:])
	return s, e
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

// parseNameListStmt parses `keep`/`drop <vars...>;`, expanding numbered ranges
// (`keep x1-x5;`).
func (p *Parser) parseNameListStmt(kw string) ast.Statement {
	p.next() // the keyword
	var vars []string
	for p.curIs(lexer.IDENT) {
		lo := p.cur.Literal
		p.next()
		if p.curIs(lexer.MINUS) && p.peek.Type == lexer.IDENT {
			p.next() // consume '-'
			hi := p.cur.Literal
			p.next()
			vars = append(vars, expandVarRange(lo, hi)...)
			continue
		}
		vars = append(vars, lo)
	}
	p.expectSemicolon()
	if kw == "keep" {
		return &ast.KeepStatement{Vars: vars}
	}
	return &ast.DropStatement{Vars: vars}
}

// parseRenameStmt parses `rename old=new old2=new2 ...;` (no parentheses, unlike
// the rename= dataset option). Keys are lowercased; new names preserve case.
func (p *Parser) parseRenameStmt() ast.Statement {
	p.next() // 'rename'
	m := map[string]string{}
	for p.curIs(lexer.IDENT) {
		old := p.cur.Literal
		p.next()
		if p.curIs(lexer.EQ) {
			p.next()
			if p.curIs(lexer.IDENT) {
				m[strings.ToLower(old)] = p.cur.Literal
				p.next()
			}
		}
	}
	p.expectSemicolon()
	return &ast.RenameStatement{Map: m}
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
	case p.identIs("label") && p.peek.Type != lexer.EQ:
		return p.parseLabelStmt()
	case p.identIs("var"):
		return &ast.VarStatement{Vars: p.parseProcNameList()}
	case p.identIs("class"):
		return &ast.ClassStatement{Vars: p.parseProcNameList()}
	case p.identIs("tables") || p.identIs("table"):
		return p.parseTables()
	case p.identIs("output"):
		return p.parseMeansOutput()
	case p.identIs("model"):
		return p.parseModel()
	case p.identIs("value"):
		return p.parseValueStmt(false)
	case p.identIs("invalue"):
		return p.parseValueStmt(true)
	default:
		return p.parseRawStatement()
	}
}

// parseProofStatement parses one assertion in a PROC PROOF step body. The
// supported kinds are require/notnull/unique (name lists), values (`var in
// (...)`), range (`var lo - hi`), and rule (`"label": <expr>`). Each may carry a
// trailing `/ severity= message=` tail — except rule, whose expression consumes
// `/` as division.
func (p *Parser) parseProofStatement() ast.Statement {
	if p.curIs(lexer.SEMICOLON) {
		p.next()
		return nil
	}
	if !p.curIs(lexer.IDENT) {
		return p.parseRawStatement()
	}
	switch strings.ToLower(p.cur.Literal) {
	case "require", "notnull", "unique":
		kind := strings.ToLower(p.cur.Literal)
		p.next()
		s := &ast.ProofStatement{Kind: kind}
		for p.curIs(lexer.IDENT) {
			s.Vars = append(s.Vars, p.cur.Literal)
			p.next()
		}
		p.parseProofTail(s)
		p.expectSemicolon()
		return s
	case "values":
		p.next()
		s := &ast.ProofStatement{Kind: "values"}
		if p.curIs(lexer.IDENT) {
			s.Vars = append(s.Vars, p.cur.Literal)
			p.next()
		}
		if p.identIs("in") {
			p.next()
		}
		if p.curIs(lexer.LPAREN) {
			p.next()
			for !p.curIs(lexer.RPAREN) && !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) {
				if p.curIs(lexer.STRING) || p.curIs(lexer.NUMBER) || p.curIs(lexer.IDENT) {
					s.Values = append(s.Values, p.cur.Literal)
				}
				p.next() // also steps over commas between items
			}
			if p.curIs(lexer.RPAREN) {
				p.next()
			}
		}
		p.parseProofTail(s)
		p.expectSemicolon()
		return s
	case "type":
		p.next()
		s := &ast.ProofStatement{Kind: "type"}
		for p.curIs(lexer.IDENT) {
			v := p.cur.Literal
			p.next()
			t := ""
			if p.curIs(lexer.EQ) {
				p.next()
				if p.curIs(lexer.IDENT) {
					t = strings.ToLower(p.cur.Literal)
					p.next()
				}
			}
			s.Vars = append(s.Vars, v)
			s.Values = append(s.Values, t)
		}
		p.parseProofTail(s)
		p.expectSemicolon()
		return s
	case "key":
		p.next()
		s := &ast.ProofStatement{Kind: "key"}
		for p.curIs(lexer.IDENT) && !p.identIs("references") { // child key columns
			s.Vars = append(s.Vars, p.cur.Literal)
			p.next()
		}
		if p.identIs("references") {
			p.next()
			if p.curIs(lexer.IDENT) {
				s.RefTable = p.cur.Literal
				p.next()
			}
			if p.curIs(lexer.LPAREN) {
				p.next()
				for !p.curIs(lexer.RPAREN) && !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) {
					if p.curIs(lexer.IDENT) {
						s.RefCols = append(s.RefCols, p.cur.Literal)
					}
					p.next() // also steps over commas between parent columns
				}
				if p.curIs(lexer.RPAREN) {
					p.next()
				}
			}
		}
		// A bare `key region` (no references) defaults the parent column list to the
		// child columns' names — harmless, and keeps single-column usage terse.
		if len(s.RefCols) == 0 {
			s.RefCols = append(s.RefCols, s.Vars...)
		}
		p.parseProofTail(s)
		p.expectSemicolon()
		return s
	case "range":
		p.next()
		s := &ast.ProofStatement{Kind: "range"}
		if p.curIs(lexer.IDENT) {
			s.Vars = append(s.Vars, p.cur.Literal)
			p.next()
		}
		if op, ok := proofRangeOp(p.cur.Type); ok { // relational form: range x >= 0
			s.Op = op
			p.next()
			s.Bound = p.parseProofNumber()
		} else { // inclusive form: range x lo - hi
			s.Low = p.parseProofNumber()
			if p.curIs(lexer.MINUS) { // the lo - hi separator
				p.next()
			}
			s.High = p.parseProofNumber()
		}
		p.parseProofTail(s)
		p.expectSemicolon()
		return s
	case "rule":
		p.next()
		s := &ast.ProofStatement{Kind: "rule"}
		if p.curIs(lexer.STRING) {
			s.Label = p.cur.Literal
			p.next()
		}
		if p.curIs(lexer.COLON) {
			p.next()
		}
		// Capture the rule expression as raw source up to the terminating `;` or a
		// `/ severity=`/`/ message=` option tail, then parse it in isolation. This
		// lets a rule expression use `/` as division without colliding with the
		// option-tail introducer (which a left-to-right expression parse cannot
		// distinguish).
		start := p.cur.Pos
		end := start
		for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
			if p.curIs(lexer.SLASH) && (p.peekIdentIs("severity") || p.peekIdentIs("message")) {
				break // start of the option tail
			}
			end = p.cur.End
			p.next()
		}
		s.Expr = ParseExpressionString(p.l.Slice(start, end))
		p.parseProofTail(s)
		p.expectSemicolon()
		return s
	default:
		return p.parseRawStatement()
	}
}

// proofRangeOp maps a comparison token to its operator text for the relational
// `range <var> <op> <num>` form. The second return is false for a non-comparison
// token (the inclusive `lo - hi` form).
func proofRangeOp(t lexer.TokenType) (string, bool) {
	switch t {
	case lexer.GE:
		return ">=", true
	case lexer.LE:
		return "<=", true
	case lexer.GT:
		return ">", true
	case lexer.LT:
		return "<", true
	case lexer.EQ:
		return "=", true
	case lexer.NE:
		return "^=", true
	}
	return "", false
}

// parseProofNumber reads an optionally-signed numeric literal (a range endpoint)
// and returns its source text, or "" if none is present.
func (p *Parser) parseProofNumber() string {
	sign := ""
	if p.curIs(lexer.MINUS) {
		sign = "-"
		p.next()
	}
	if p.curIs(lexer.NUMBER) {
		lit := sign + p.cur.Literal
		p.next()
		return lit
	}
	return sign
}

// parseProofTail consumes an optional `/ severity= message=` option tail on a
// proof assertion.
func (p *Parser) parseProofTail(s *ast.ProofStatement) {
	if !p.curIs(lexer.SLASH) {
		return
	}
	p.next() // '/'
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		name := strings.ToLower(p.cur.Literal)
		p.next()
		if !p.curIs(lexer.EQ) {
			continue
		}
		p.next()
		val := ""
		if p.curIs(lexer.STRING) || p.curIs(lexer.IDENT) || p.curIs(lexer.NUMBER) {
			val = p.cur.Literal
			p.next()
		}
		switch name {
		case "severity":
			s.Severity = strings.ToLower(val)
		case "message":
			s.Message = val
		}
	}
}

// parseValueStmt parses a PROC FORMAT `value [$]name <range>=<label> ...;`
// statement into a ValueStatement. Ranges may be single values, `low`/`high`
// open-ended or exclusive (`a <- b`, `a -< b`) intervals, comma lists (each
// value shares the label), or the catch-all `other`.
func (p *Parser) parseValueStmt(invalue bool) ast.Statement {
	p.next() // 'value' / 'invalue'
	stmt := &ast.ValueStatement{Invalue: invalue}
	if p.curIs(lexer.DOLLAR) {
		stmt.Char = true
		stmt.Name = "$"
		p.next()
	}
	if p.curIs(lexer.IDENT) {
		stmt.Name += p.cur.Literal
		p.next()
	}

	var pending []ast.ValueRange
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		switch {
		case p.curIs(lexer.COMMA):
			p.next()
		case p.curIs(lexer.EQ):
			p.next()
			label := ""
			if p.curIs(lexer.STRING) || p.curIs(lexer.NUMBER) || p.curIs(lexer.IDENT) {
				label = p.cur.Literal
				p.next()
			}
			for i := range pending {
				pending[i].Label = label
			}
			stmt.Ranges = append(stmt.Ranges, pending...)
			pending = nil
		default:
			r, ok := p.parseRangeItem()
			if !ok {
				p.next() // skip an unexpected token to avoid looping
				continue
			}
			pending = append(pending, r)
		}
	}
	p.expectSemicolon()
	return stmt
}

// parseRangeItem parses a single range (one endpoint, or `lo - hi`, or `other`).
func (p *Parser) parseRangeItem() (ast.ValueRange, bool) {
	var r ast.ValueRange
	if p.identIs("other") {
		r.Other = true
		p.next()
		return r, true
	}
	lo, noLo, ok := p.parseEndpoint()
	if !ok {
		return r, false
	}
	r.Low, r.NoLow = lo, noLo
	if p.curIs(lexer.LT) { // exclusive low: `a <- b`
		r.LowExcl = true
		p.next()
	}
	if p.curIs(lexer.MINUS) {
		p.next()
		if p.curIs(lexer.LT) { // exclusive high: `a -< b`
			r.HighExcl = true
			p.next()
		}
		hi, noHi, ok := p.parseEndpoint()
		if !ok {
			return r, false
		}
		r.High, r.NoHigh = hi, noHi
	} else { // single value
		r.High, r.NoHigh = r.Low, r.NoLow
	}
	return r, true
}

// parseEndpoint parses one range endpoint: a number (possibly negative), a
// quoted string, or the `low`/`high` keyword (returned as none=true).
func (p *Parser) parseEndpoint() (val string, none, ok bool) {
	switch {
	case p.identIs("low") || p.identIs("high"):
		p.next()
		return "", true, true
	case p.curIs(lexer.STRING) || p.curIs(lexer.NUMBER):
		v := p.cur.Literal
		p.next()
		return v, false, true
	case p.curIs(lexer.MINUS) && p.peek.Type == lexer.NUMBER:
		p.next() // '-'
		v := "-" + p.cur.Literal
		p.next()
		return v, false, true
	}
	return "", false, false
}

// parseModel parses `model <response> = <predictor ...>;`.
func (p *Parser) parseModel() ast.Statement {
	p.next() // 'model'
	stmt := &ast.ModelStatement{}
	if p.curIs(lexer.IDENT) {
		stmt.Response = p.cur.Literal
		p.next()
	}
	if p.curIs(lexer.EQ) {
		p.next()
	}
	for p.curIs(lexer.IDENT) {
		stmt.Predictors = append(stmt.Predictors, p.cur.Literal)
		p.next()
	}
	p.expectSemicolon()
	return stmt
}

// parseTables parses a PROC FREQ `tables <request ...> [/ options];` statement.
// A request is one or more variables joined with `*` (e.g. `a` one-way,
// `a*b` two-way). Space-separated requests are independent. Any `/ options`
// tail is skipped.
func (p *Parser) parseTables() ast.Statement {
	p.next() // 'tables'/'table'
	stmt := &ast.TablesStatement{}
	var cur []string
	for p.curIs(lexer.IDENT) || p.curIs(lexer.STAR) {
		if p.curIs(lexer.STAR) {
			p.next()
			continue
		}
		cur = append(cur, p.cur.Literal)
		stmt.Vars = append(stmt.Vars, p.cur.Literal)
		p.next()
		if !p.curIs(lexer.STAR) { // request ends unless the next token crosses it
			stmt.Requests = append(stmt.Requests, cur)
			cur = nil
		}
	}
	if len(cur) > 0 {
		stmt.Requests = append(stmt.Requests, cur)
	}
	// Collect any trailing `/ options` (lowercased identifiers) up to the
	// terminating semicolon. A `/` introduces the option list; option values
	// (`opt=...`) are recorded by name only.
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		if p.curIs(lexer.IDENT) {
			name := strings.ToLower(p.cur.Literal)
			if name == "out" && p.peek.Type == lexer.EQ {
				p.next() // '='
				p.next() // dataset name
				if p.curIs(lexer.IDENT) {
					stmt.Out = p.cur.Literal
				}
			} else {
				stmt.Options = append(stmt.Options, name)
			}
		}
		p.next()
	}
	p.expectSemicolon()
	return stmt
}

// parseMeansOutput parses PROC MEANS/SUMMARY
// `output out=<name> <stat>=<v ...> ... [/ options];`. `out=` names the dataset;
// each `stat=name name ...` clause names the output variables for a statistic,
// positional to the analysis (VAR) variables. A trailing `/ options` tail is
// skipped.
func (p *Parser) parseMeansOutput() ast.Statement {
	p.next() // 'output'
	stmt := &ast.MeansOutputStatement{}
	for p.curIs(lexer.IDENT) {
		key := strings.ToLower(p.cur.Literal)
		if p.peek.Type != lexer.EQ {
			p.next() // stray token; skip
			continue
		}
		p.next() // key
		p.next() // '='
		if key == "out" {
			if p.curIs(lexer.IDENT) {
				stmt.Out = p.cur.Literal
				p.next()
			}
			continue
		}
		// A statistic keyword: collect output names until the next `key=` or the end.
		var names []string
		for p.curIs(lexer.IDENT) && p.peek.Type != lexer.EQ {
			names = append(names, p.cur.Literal)
			p.next()
		}
		stmt.Stats = append(stmt.Stats, ast.MeansOutStat{Stat: key, Names: names})
	}
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		p.next() // skip any `/ options` tail
	}
	p.expectSemicolon()
	return stmt
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

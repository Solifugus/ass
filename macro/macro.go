// Package macro implements the SAS macro preprocessor. It runs as a text pass
// BEFORE the lexer: it maintains a macro-variable symbol table and a macro
// definition table, resolves &var references, and expands %let/%macro/%if/%do
// constructs, producing ordinary SAS source for the parser.
package macro

import (
	"strconv"
	"strings"
)

// Processor holds macro state across a single Process call.
type Processor struct {
	vars   map[string]string
	macros map[string]*macroDef
}

type macroDef struct {
	params   []string          // positional parameter names, in order
	defaults map[string]string // default values for named params
	body     string            // raw macro body text
}

// New creates an empty Processor.
func New() *Processor {
	return &Processor{
		vars:   make(map[string]string),
		macros: make(map[string]*macroDef),
	}
}

// Process expands all macro constructs in src and returns ordinary SAS source.
func Process(src string) string {
	return New().Process(src)
}

// Process expands src using (and updating) this processor's state.
func (p *Processor) Process(src string) string {
	var out strings.Builder
	c := &cursor{s: []rune(src)}
	p.expandInto(c, &out, nil)
	return out.String()
}

// cursor is a rune-indexed scan position over the input.
type cursor struct {
	s []rune
	i int
}

func (c *cursor) eof() bool { return c.i >= len(c.s) }
func (c *cursor) cur() rune {
	if c.eof() {
		return 0
	}
	return c.s[c.i]
}

// expandInto scans from c, writing expanded text to out, until EOF or until it
// reaches a top-level %keyword present in stops (which it leaves unconsumed for
// the caller). It returns the stop keyword reached, or "" at EOF.
func (p *Processor) expandInto(c *cursor, out *strings.Builder, stops map[string]bool) string {
	for !c.eof() {
		r := c.cur()
		if r == '&' {
			text, ni := resolveAmp(c.s, c.i, p.vars)
			out.WriteString(text)
			c.i = ni
			continue
		}
		if kw := keywordAt(c); kw != "" {
			if stops[kw] {
				return kw
			}
			switch kw {
			case "let":
				consumeKeyword(c)
				p.doLet(c)
			case "macro":
				consumeKeyword(c)
				p.doMacro(c)
			case "if":
				consumeKeyword(c)
				p.doIf(c, out)
			case "do":
				consumeKeyword(c)
				p.doDo(c, out)
			case "mend", "end", "else", "then", "to", "by":
				consumeKeyword(c) // stray control word; drop it
			default:
				if _, ok := p.macros[kw]; ok {
					consumeKeyword(c)
					p.doCall(c, out, kw)
				} else {
					// Not a macro: emit '%' literally and let the word scan as text.
					out.WriteRune('%')
					c.i++
				}
			}
			continue
		}
		out.WriteRune(r)
		c.i++
	}
	return ""
}

// doLet handles `%let name = value ;` (name already past `%let`).
func (p *Processor) doLet(c *cursor) {
	skipSpaces(c)
	name := strings.ToLower(readIdent(c))
	skipSpaces(c)
	if c.cur() == '=' {
		c.i++
	}
	value := readUntil(c, ';')
	if c.cur() == ';' {
		c.i++
	}
	p.vars[name] = strings.TrimSpace(resolveAmpsStr(value, p.vars))
}

// doMacro handles `%macro name(params); body %mend [name];`.
func (p *Processor) doMacro(c *cursor) {
	skipSpaces(c)
	name := strings.ToLower(readIdent(c))
	def := &macroDef{defaults: make(map[string]string)}
	skipSpaces(c)
	if c.cur() == '(' {
		c.i++ // '('
		params := readUntil(c, ')')
		if c.cur() == ')' {
			c.i++
		}
		for _, raw := range strings.Split(params, ",") {
			part := strings.TrimSpace(raw)
			if part == "" {
				continue
			}
			if eq := strings.IndexByte(part, '='); eq >= 0 {
				pn := strings.ToLower(strings.TrimSpace(part[:eq]))
				def.params = append(def.params, pn)
				def.defaults[pn] = strings.TrimSpace(part[eq+1:])
			} else {
				def.params = append(def.params, strings.ToLower(part))
			}
		}
	}
	// Consume the rest of the %macro statement up to its ';'.
	readUntil(c, ';')
	if c.cur() == ';' {
		c.i++
	}
	def.body = captureUntilMend(c)
	// Consume `%mend [name] ;`.
	if keywordAt(c) == "mend" {
		consumeKeyword(c)
		readUntil(c, ';')
		if c.cur() == ';' {
			c.i++
		}
	}
	p.macros[name] = def
}

// doCall expands an invocation of macro `name` (already past `%name`), binding
// any arguments to the macro's parameters for the duration of the expansion.
func (p *Processor) doCall(c *cursor, out *strings.Builder, name string) {
	def := p.macros[name]
	var args []string
	skipSpaces(c)
	if c.cur() == '(' {
		c.i++
		argText := readUntil(c, ')')
		if c.cur() == ')' {
			c.i++
		}
		args = splitArgs(argText)
	}

	// Classify arguments into positional (no '=') and keyword (name=value).
	var posArgs []string
	kwArgs := make(map[string]string)
	for _, a := range args {
		if eq := strings.IndexByte(a, '='); eq >= 0 {
			kwArgs[strings.ToLower(strings.TrimSpace(a[:eq]))] = strings.TrimSpace(a[eq+1:])
		} else {
			posArgs = append(posArgs, a)
		}
	}

	// Bind parameters, saving and restoring any shadowed macro variables so the
	// binding is scoped to this call.
	saved := make(map[string]*string)
	bind := func(k, v string) {
		k = strings.ToLower(k)
		if old, ok := p.vars[k]; ok {
			oc := old
			saved[k] = &oc
		} else {
			saved[k] = nil
		}
		p.vars[k] = v
	}
	posIdx := 0
	for _, pname := range def.params {
		_, isKeyword := def.defaults[pname]
		val := def.defaults[pname]
		if v, ok := kwArgs[pname]; ok {
			val = v
		} else if !isKeyword {
			if posIdx < len(posArgs) {
				val = posArgs[posIdx]
			}
			posIdx++
		}
		bind(pname, strings.TrimSpace(resolveAmpsStr(val, p.vars)))
	}

	sub := &cursor{s: []rune(def.body)}
	p.expandInto(sub, out, nil)

	for k, v := range saved {
		if v == nil {
			delete(p.vars, k)
		} else {
			p.vars[k] = *v
		}
	}
}

// doDo handles `%do;` (simple) and `%do v = a %to b [%by s];` iterative blocks
// (already past `%do`).
func (p *Processor) doDo(c *cursor, out *strings.Builder) {
	skipSpaces(c)
	if c.cur() == ';' {
		c.i++
		body := captureBlock(c)
		p.expandBody(body, out)
		return
	}
	v := strings.ToLower(readIdent(c))
	skipSpaces(c)
	if c.cur() == '=' {
		c.i++
	}
	from := atoiOr(strings.TrimSpace(resolveAmpsStr(readUntilKeyword(c, "to"), p.vars)), 0)
	if keywordAt(c) == "to" {
		consumeKeyword(c)
	}
	step := 1
	toText := readUntilKeywordOrChar(c, "by", ';')
	to := atoiOr(strings.TrimSpace(resolveAmpsStr(toText, p.vars)), 0)
	if keywordAt(c) == "by" {
		consumeKeyword(c)
		step = atoiOr(strings.TrimSpace(resolveAmpsStr(readUntil(c, ';'), p.vars)), 1)
	}
	if c.cur() == ';' {
		c.i++
	}
	body := captureBlock(c)
	if step == 0 {
		return
	}
	for i := from; (step > 0 && i <= to) || (step < 0 && i >= to); i += step {
		p.vars[v] = strconv.Itoa(i)
		p.expandBody(body, out)
	}
}

// doIf handles `%if cond %then <branch> [%else <branch>]`, where a branch is a
// `%do ... %end` block or a single statement ending at `;`.
func (p *Processor) doIf(c *cursor, out *strings.Builder) {
	cond := evalCond(strings.TrimSpace(resolveAmpsStr(readUntilKeyword(c, "then"), p.vars)))
	if keywordAt(c) == "then" {
		consumeKeyword(c)
	}
	thenBody := p.readBranch(c)
	var elseBody string
	skipSpaces(c)
	if keywordAt(c) == "else" {
		consumeKeyword(c)
		elseBody = p.readBranch(c)
	}
	if cond {
		p.expandBody(thenBody, out)
	} else {
		p.expandBody(elseBody, out)
	}
}

// readBranch reads one %if/%else branch: a %do block (body) or a single
// statement up to and including its ';'.
func (p *Processor) readBranch(c *cursor) string {
	skipSpaces(c)
	if keywordAt(c) == "do" {
		consumeKeyword(c)
		if c.cur() == ';' {
			c.i++
		}
		body := captureBlock(c)
		if c.cur() == ';' { // consume the ';' terminating `%end;`
			c.i++
		}
		return body
	}
	body := readUntil(c, ';')
	if c.cur() == ';' {
		c.i++
		body += ";"
	}
	return body
}

// expandBody expands a captured raw body string into out.
func (p *Processor) expandBody(body string, out *strings.Builder) {
	sub := &cursor{s: []rune(body)}
	p.expandInto(sub, out, nil)
}

// --- scanning helpers ---

func isAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}
func isIdentChar(r rune) bool { return isAlpha(r) || (r >= '0' && r <= '9') }

func skipSpaces(c *cursor) {
	for !c.eof() {
		switch c.cur() {
		case ' ', '\t', '\r', '\n':
			c.i++
		default:
			return
		}
	}
}

func readIdent(c *cursor) string {
	start := c.i
	for !c.eof() && isIdentChar(c.cur()) {
		c.i++
	}
	return string(c.s[start:c.i])
}

// readUntil reads raw text up to (not including) the delimiter rune.
func readUntil(c *cursor, delim rune) string {
	start := c.i
	for !c.eof() && c.cur() != delim {
		c.i++
	}
	return string(c.s[start:c.i])
}

// readUntilKeyword reads raw text up to (not including) the given %keyword.
func readUntilKeyword(c *cursor, kw string) string {
	start := c.i
	for !c.eof() && keywordAt(c) != kw {
		c.i++
	}
	return string(c.s[start:c.i])
}

// readUntilKeywordOrChar reads raw text up to the given %keyword or the rune.
func readUntilKeywordOrChar(c *cursor, kw string, ch rune) string {
	start := c.i
	for !c.eof() && keywordAt(c) != kw && c.cur() != ch {
		c.i++
	}
	return string(c.s[start:c.i])
}

// keywordAt returns the lowercased word of a `%word` at the cursor, without
// consuming it; "" if the cursor is not at a `%word`.
func keywordAt(c *cursor) string {
	if c.cur() != '%' || c.i+1 >= len(c.s) || !isAlpha(c.s[c.i+1]) {
		return ""
	}
	j := c.i + 1
	for j < len(c.s) && isIdentChar(c.s[j]) {
		j++
	}
	return strings.ToLower(string(c.s[c.i+1 : j]))
}

// consumeKeyword advances past a `%word` and returns the lowercased word.
func consumeKeyword(c *cursor) string {
	kw := keywordAt(c)
	c.i++ // '%'
	for !c.eof() && isIdentChar(c.cur()) {
		c.i++
	}
	return kw
}

// captureUntilMend returns raw text up to the matching %mend, leaving the cursor
// at %mend. (Nested %macro definitions are not supported.)
func captureUntilMend(c *cursor) string {
	start := c.i
	for !c.eof() && keywordAt(c) != "mend" {
		if keywordAt(c) != "" {
			consumeKeyword(c)
		} else {
			c.i++
		}
	}
	return string(c.s[start:c.i])
}

// captureBlock returns raw text up to the matching %end (tracking nested %do),
// consuming that %end.
func captureBlock(c *cursor) string {
	start := c.i
	depth := 0
	for !c.eof() {
		switch keywordAt(c) {
		case "do":
			depth++
			consumeKeyword(c)
		case "end":
			if depth == 0 {
				body := string(c.s[start:c.i])
				consumeKeyword(c)
				return body
			}
			depth--
			consumeKeyword(c)
		case "":
			c.i++
		default:
			consumeKeyword(c)
		}
	}
	return string(c.s[start:])
}

// splitArgs splits a macro argument list on top-level commas.
func splitArgs(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var args []string
	depth := 0
	start := 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	args = append(args, strings.TrimSpace(s[start:]))
	return args
}

// resolveAmp resolves a single &name (or &&… collapsed) reference at rs[i]
// (rs[i]=='&'), returning the replacement text and the index past the reference
// (including a single trailing '.' terminator if present).
func resolveAmp(rs []rune, i int, vars map[string]string) (string, int) {
	i++ // skip first '&'
	for i < len(rs) && rs[i] == '&' { // collapse &&… (basic)
		i++
	}
	j := i
	for j < len(rs) && isIdentChar(rs[j]) {
		j++
	}
	name := strings.ToLower(string(rs[i:j]))
	end := j
	if end < len(rs) && rs[end] == '.' { // trailing dot terminator is consumed
		end++
	}
	if v, ok := vars[name]; ok {
		return v, end
	}
	return "&" + string(rs[i:j]), end // unknown: leave a single & + name
}

// resolveAmpsStr resolves every &name reference in a string.
func resolveAmpsStr(s string, vars map[string]string) string {
	rs := []rune(s)
	var b strings.Builder
	for i := 0; i < len(rs); {
		if rs[i] == '&' && i+1 < len(rs) && (isAlpha(rs[i+1]) || rs[i+1] == '&') {
			text, ni := resolveAmp(rs, i, vars)
			b.WriteString(text)
			i = ni
			continue
		}
		b.WriteRune(rs[i])
		i++
	}
	return b.String()
}

// evalCond evaluates a (already &-resolved) macro condition. It supports the
// symbolic and word comparison operators; with no operator it is true when the
// text is non-empty and not "0".
func evalCond(expr string) bool {
	ops := []struct {
		tok string
		f   func(int) bool
	}{
		{">=", func(c int) bool { return c >= 0 }},
		{"<=", func(c int) bool { return c <= 0 }},
		{"^=", func(c int) bool { return c != 0 }},
		{"ne", func(c int) bool { return c != 0 }},
		{"eq", func(c int) bool { return c == 0 }},
		{"ge", func(c int) bool { return c >= 0 }},
		{"le", func(c int) bool { return c <= 0 }},
		{"gt", func(c int) bool { return c > 0 }},
		{"lt", func(c int) bool { return c < 0 }},
		{"=", func(c int) bool { return c == 0 }},
		{">", func(c int) bool { return c > 0 }},
		{"<", func(c int) bool { return c < 0 }},
	}
	for _, op := range ops {
		if idx := findOp(expr, op.tok); idx >= 0 {
			left := strings.TrimSpace(expr[:idx])
			right := strings.TrimSpace(expr[idx+len(op.tok):])
			return op.f(compareStr(left, right))
		}
	}
	expr = strings.TrimSpace(expr)
	return expr != "" && expr != "0"
}

// findOp finds a comparison operator. Word operators must be space-delimited;
// symbolic ones match anywhere.
func findOp(expr, op string) int {
	if isAlpha(rune(op[0])) {
		padded := " " + op + " "
		if i := strings.Index(strings.ToLower(expr), padded); i >= 0 {
			return i + 1
		}
		return -1
	}
	return strings.Index(expr, op)
}

// compareStr compares two operands numerically when both parse as numbers,
// otherwise lexically; returns -1/0/1.
func compareStr(a, b string) int {
	af, aerr := strconv.ParseFloat(a, 64)
	bf, berr := strconv.ParseFloat(b, 64)
	if aerr == nil && berr == nil {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(a, b)
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
		return int(f)
	}
	return def
}

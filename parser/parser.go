// Package parser turns a token stream from the lexer into an abstract syntax
// tree (see package ast). It splits a program into DATA and PROC steps and
// parses statements and expressions within each step.
package parser

import (
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/lexer"
)

// Parser builds an *ast.Program from SAS source. It keeps a one-token lookahead
// (cur/peek).
type Parser struct {
	l    *lexer.Lexer
	cur  lexer.Token
	peek lexer.Token

	errors []string
}

// New creates a Parser over the given SAS source.
func New(input string) *Parser {
	p := &Parser{l: lexer.New(input)}
	p.next() // load peek
	p.next() // load cur
	return p
}

// Errors returns any errors accumulated during parsing.
func (p *Parser) Errors() []string { return p.errors }

func (p *Parser) next() {
	p.cur = p.peek
	p.peek = p.l.NextToken()
}

func (p *Parser) curIs(t lexer.TokenType) bool  { return p.cur.Type == t }
func (p *Parser) addError(msg string)           { p.errors = append(p.errors, msg) }

// ParseProgram parses the whole input into a Program. Tokens outside of a DATA
// or PROC step (e.g. stray macro invocations not yet handled) are skipped.
func (p *Parser) ParseProgram() *ast.Program {
	prog := &ast.Program{}
	for !p.curIs(lexer.EOF) {
		switch p.cur.Type {
		case lexer.DATA:
			prog.Steps = append(prog.Steps, p.parseDataStep())
		case lexer.PROC:
			prog.Steps = append(prog.Steps, p.parseProcStep())
		default:
			p.next() // skip anything not starting a step
		}
	}
	return prog
}

// parseDataStep parses `data <names...>; <body> run;`.
func (p *Parser) parseDataStep() ast.Step {
	p.next() // consume DATA
	ds := &ast.DataStep{}
	for p.curIs(lexer.IDENT) {
		ds.Datasets = append(ds.Datasets, p.cur.Literal)
		p.next()
	}
	p.expectSemicolon()
	ds.Body = p.parseStepBody(p.parseDataStatement)
	return ds
}

// parseProcStep parses `proc <name> <options>; <body> run|quit;`.
func (p *Parser) parseProcStep() ast.Step {
	p.next() // consume PROC
	ps := &ast.ProcStep{}
	if p.curIs(lexer.IDENT) {
		ps.Name = strings.ToLower(p.cur.Literal)
		p.next()
	}
	// Options up to the statement-terminating semicolon. Note: an option name
	// may arrive as a keyword token (e.g. `data=`), so we use the literal.
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		name := strings.ToLower(p.cur.Literal)
		p.next()
		if p.curIs(lexer.EQ) {
			p.next()
			value := p.cur.Literal
			p.next()
			if name == "data" {
				ps.Data = value
			} else {
				ps.Options = append(ps.Options, ast.ProcOption{Name: name, Value: value})
			}
		} else {
			ps.Options = append(ps.Options, ast.ProcOption{Name: name})
		}
	}
	p.expectSemicolon()
	ps.Body = p.parseStepBody(p.parseProcStatement)
	return ps
}

// parseStepBody collects statements (using parseStmt) until a RUN/QUIT
// terminator (or EOF) and consumes that terminator and its semicolon.
func (p *Parser) parseStepBody(parseStmt func() ast.Statement) []ast.Statement {
	var body []ast.Statement
	for !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) && !p.curIs(lexer.EOF) {
		if stmt := parseStmt(); stmt != nil {
			body = append(body, stmt)
		}
	}
	if p.curIs(lexer.RUN) || p.curIs(lexer.QUIT) {
		p.next()
		p.expectSemicolon()
	}
	return body
}

// parseDatalines consumes `datalines; <raw block> ;` into a DatalinesStatement.
func (p *Parser) parseDatalines() ast.Statement {
	p.next() // consume DATALINES keyword
	p.expectSemicolon()
	stmt := &ast.DatalinesStatement{}
	if p.curIs(lexer.DATALINES_DATA) {
		if p.cur.Literal != "" {
			stmt.Lines = strings.Split(p.cur.Literal, "\n")
		}
		p.next()
	}
	p.expectSemicolon() // the terminator line
	return stmt
}

// parseRawStatement gathers token literals up to the next semicolon into a
// RawStatement, so unrecognized constructs are preserved rather than dropped.
func (p *Parser) parseRawStatement() ast.Statement {
	var parts []string
	for !p.curIs(lexer.SEMICOLON) && !p.curIs(lexer.EOF) && !p.curIs(lexer.RUN) && !p.curIs(lexer.QUIT) {
		parts = append(parts, p.cur.Literal)
		p.next()
	}
	p.expectSemicolon()
	if len(parts) == 0 {
		return nil
	}
	return &ast.RawStatement{Text: strings.Join(parts, " ")}
}

// expectSemicolon consumes a semicolon if present, recording an error otherwise.
func (p *Parser) expectSemicolon() {
	if p.curIs(lexer.SEMICOLON) {
		p.next()
		return
	}
	p.addError("expected ';' at line " + itoa(p.cur.Line))
}

// itoa is a tiny strconv.Itoa to avoid importing strconv for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

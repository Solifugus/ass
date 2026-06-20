package parser

import (
	"strconv"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/formats"
	"github.com/solifugus/ass/lexer"
)

// Operator precedence levels, lowest to highest. POWER is above PREFIX so that
// -2**2 parses as -(2**2), matching SAS.
const (
	_ int = iota
	pLOWEST
	pOR      // | or
	pAND     // & and
	pCOMPARE // = ^= < <= > >= (and word forms)
	pCONCAT  // ||
	pSUM     // + -
	pPRODUCT // * /
	pPREFIX  // unary - +
	pPOWER   // **
)

// parseExpression parses an expression with operators binding at least as
// tightly as minPrec (precedence-climbing / Pratt).
func (p *Parser) parseExpression(minPrec int) ast.Expression {
	left := p.parsePrefixExpr()
	if left == nil {
		return nil
	}
	for {
		op, prec, ok := p.infixInfo()
		if !ok || minPrec >= prec {
			break
		}
		left = p.parseInfixExpr(left, op, prec)
	}
	return left
}

// parsePrefixExpr parses a primary expression or a prefix-operator expression.
func (p *Parser) parsePrefixExpr() ast.Expression {
	switch p.cur.Type {
	case lexer.NUMBER:
		lit := p.cur.Literal
		v, err := strconv.ParseFloat(lit, 64)
		if err != nil {
			p.addError("invalid number " + lit + " at line " + itoa(p.cur.Line))
		}
		p.next()
		return &ast.NumberLiteral{Literal: lit, Value: v}
	case lexer.STRING:
		// Date/time/datetime literal: a string immediately followed by a `d`, `t`,
		// or `dt` suffix, e.g. '01JAN2020'd, '14:30:00't, '01JAN2020:14:30:00'dt.
		if p.peek.Type == lexer.IDENT && p.peek.Pos == p.cur.End {
			switch strings.ToLower(p.peek.Literal) {
			case "d":
				if day, ok := formats.ParseDateLiteral(p.cur.Literal); ok {
					lit := "'" + p.cur.Literal + "'d"
					p.next() // string
					p.next() // 'd'
					return &ast.NumberLiteral{Literal: lit, Value: day}
				}
			case "t":
				if sec, ok := formats.ParseTimeLiteral(p.cur.Literal); ok {
					lit := "'" + p.cur.Literal + "'t"
					p.next() // string
					p.next() // 't'
					return &ast.NumberLiteral{Literal: lit, Value: sec}
				}
			case "dt":
				if sec, ok := formats.ParseDatetimeLiteral(p.cur.Literal); ok {
					lit := "'" + p.cur.Literal + "'dt"
					p.next() // string
					p.next() // 'dt'
					return &ast.NumberLiteral{Literal: lit, Value: sec}
				}
			}
		}
		s := &ast.StringLiteral{Value: p.cur.Literal}
		p.next()
		return s
	case lexer.DOT:
		p.next()
		return &ast.MissingLiteral{}
	case lexer.MINUS, lexer.PLUS:
		op := p.cur.Literal
		p.next()
		right := p.parseExpression(pPREFIX)
		return &ast.PrefixExpression{Op: op, Right: right}
	case lexer.NOT: // ^ or ~
		p.next()
		right := p.parseExpression(pAND) // NOT binds looser than comparison
		return &ast.PrefixExpression{Op: "not", Right: right}
	case lexer.LPAREN:
		p.next()
		e := p.parseExpression(pLOWEST)
		if p.curIs(lexer.RPAREN) {
			p.next()
		} else {
			p.addError("expected ')' at line " + itoa(p.cur.Line))
		}
		return e
	case lexer.IDENT:
		lit := p.cur.Literal
		if strings.ToLower(lit) == "not" {
			p.next()
			right := p.parseExpression(pAND)
			return &ast.PrefixExpression{Op: "not", Right: right}
		}
		p.next()
		if p.curIs(lexer.LPAREN) {
			return p.parseCall(lit)
		}
		if p.curIs(lexer.LBRACE) || p.curIs(lexer.LBRACKET) {
			return p.parseArrayRef(lit)
		}
		// BY-group automatic variables: first.<var> / last.<var>.
		low := strings.ToLower(lit)
		if (low == "first" || low == "last") && p.curIs(lexer.DOT) && p.peek.Type == lexer.IDENT {
			p.next() // '.'
			name := lit + "." + p.cur.Literal
			p.next()
			return &ast.Identifier{Name: name}
		}
		return &ast.Identifier{Name: lit}
	default:
		p.addError("unexpected token " + string(p.cur.Type) + " (" + p.cur.Literal + ") in expression at line " + itoa(p.cur.Line))
		p.next()
		return nil
	}
}

// parseArrayRef parses a subscripted array reference `name{expr}` or
// `name[expr]`; cur is the opening bracket/brace.
func (p *Parser) parseArrayRef(name string) ast.Expression {
	close := lexer.RBRACE
	if p.curIs(lexer.LBRACKET) {
		close = lexer.RBRACKET
	}
	p.next() // consume opening bracket
	idx := p.parseExpression(pLOWEST)
	if p.curIs(close) {
		p.next()
	} else {
		p.addError("expected closing array subscript at line " + itoa(p.cur.Line))
	}
	return &ast.ArrayRef{Name: name, Index: idx}
}

// parseCall parses a function call argument list; cur is the '('.
func (p *Parser) parseCall(name string) ast.Expression {
	p.next() // consume '('
	call := &ast.CallExpression{Func: name}
	if p.curIs(lexer.RPAREN) {
		p.next()
		return call
	}
	call.Args = append(call.Args, p.parseExpression(pLOWEST))
	for p.curIs(lexer.COMMA) {
		p.next()
		call.Args = append(call.Args, p.parseExpression(pLOWEST))
	}
	if p.curIs(lexer.RPAREN) {
		p.next()
	} else {
		p.addError("expected ')' to close call to " + name + " at line " + itoa(p.cur.Line))
	}
	return call
}

// infixInfo inspects the current token and reports the canonical operator
// string and its precedence if it is an infix operator. Mnemonic word operators
// (and, or, eq, ne, lt, le, gt, ge) arrive as IDENT and are normalized here to
// their symbolic form so the evaluator handles a single representation.
func (p *Parser) infixInfo() (op string, prec int, ok bool) {
	switch p.cur.Type {
	case lexer.PIPE:
		return "or", pOR, true
	case lexer.AMP:
		return "and", pAND, true
	case lexer.EQ:
		return "=", pCOMPARE, true
	case lexer.NE:
		return "^=", pCOMPARE, true
	case lexer.LT:
		return "<", pCOMPARE, true
	case lexer.LE:
		return "<=", pCOMPARE, true
	case lexer.GT:
		return ">", pCOMPARE, true
	case lexer.GE:
		return ">=", pCOMPARE, true
	case lexer.CONCAT:
		return "||", pCONCAT, true
	case lexer.PLUS:
		return "+", pSUM, true
	case lexer.MINUS:
		return "-", pSUM, true
	case lexer.STAR:
		return "*", pPRODUCT, true
	case lexer.SLASH:
		return "/", pPRODUCT, true
	case lexer.POW:
		return "**", pPOWER, true
	case lexer.IDENT:
		switch strings.ToLower(p.cur.Literal) {
		case "and":
			return "and", pAND, true
		case "or":
			return "or", pOR, true
		case "eq":
			return "=", pCOMPARE, true
		case "ne":
			return "^=", pCOMPARE, true
		case "lt":
			return "<", pCOMPARE, true
		case "le":
			return "<=", pCOMPARE, true
		case "gt":
			return ">", pCOMPARE, true
		case "ge":
			return ">=", pCOMPARE, true
		}
	}
	return "", pLOWEST, false
}

// parseInfixExpr consumes the current infix operator and parses its right
// operand. ** is right-associative.
func (p *Parser) parseInfixExpr(left ast.Expression, op string, prec int) ast.Expression {
	p.next() // consume the operator token
	rightPrec := prec
	if op == "**" {
		rightPrec = prec - 1 // right associative
	}
	right := p.parseExpression(rightPrec)
	return &ast.InfixExpression{Left: left, Op: op, Right: right}
}

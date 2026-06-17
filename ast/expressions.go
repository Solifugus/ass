package ast

import "strings"

// NumberLiteral is a numeric constant. Literal preserves the source text; Value
// is its parsed numeric value.
type NumberLiteral struct {
	Literal string
	Value   float64
}

func (n *NumberLiteral) expressionNode() {}
func (n *NumberLiteral) String() string  { return n.Literal }

// StringLiteral is a character constant (already unquoted by the lexer).
type StringLiteral struct {
	Value string
}

func (s *StringLiteral) expressionNode() {}
func (s *StringLiteral) String() string  { return "'" + s.Value + "'" }

// MissingLiteral is the SAS missing value written as `.` in an expression.
type MissingLiteral struct{}

func (m *MissingLiteral) expressionNode() {}
func (m *MissingLiteral) String() string  { return "." }

// Identifier is a variable (or automatic variable) reference.
type Identifier struct {
	Name string
}

func (i *Identifier) expressionNode() {}
func (i *Identifier) String() string  { return i.Name }

// PrefixExpression is a unary operation, e.g. `-x` or `not done`.
type PrefixExpression struct {
	Op    string
	Right Expression
}

func (p *PrefixExpression) expressionNode() {}
func (p *PrefixExpression) String() string {
	// Word operators (e.g. "not") need a separating space; symbol operators
	// (e.g. "-") do not.
	sep := ""
	if len(p.Op) > 0 && isLetter(p.Op[0]) {
		sep = " "
	}
	return "(" + p.Op + sep + str(p.Right) + ")"
}

func isLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// InfixExpression is a binary operation, e.g. `a + b` or `age >= 18`.
type InfixExpression struct {
	Left  Expression
	Op    string
	Right Expression
}

func (ie *InfixExpression) expressionNode() {}
func (ie *InfixExpression) String() string {
	return "(" + str(ie.Left) + " " + ie.Op + " " + str(ie.Right) + ")"
}

// CallExpression is a function call, e.g. `substr(name, 1, 3)`.
type CallExpression struct {
	Func string
	Args []Expression
}

func (c *CallExpression) expressionNode() {}
func (c *CallExpression) String() string {
	parts := make([]string, len(c.Args))
	for i, a := range c.Args {
		parts[i] = str(a)
	}
	return c.Func + "(" + strings.Join(parts, ", ") + ")"
}

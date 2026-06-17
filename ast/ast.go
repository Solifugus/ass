// Package ast defines the abstract syntax tree node types for SAS programs:
// the program and step containers, DATA step statements, PROC steps, and
// expression nodes.
package ast

import "strings"

// Node is the interface implemented by every AST node. String renders the node
// back to a SAS-like textual form, primarily for debugging and tests.
type Node interface {
	String() string
}

// str renders a node, tolerating nil children (which can occur in partial trees
// produced when parsing encounters errors).
func str(n Node) string {
	if n == nil {
		return "<?>"
	}
	return n.String()
}

// Statement is a node that appears in the body of a step (e.g. an assignment).
type Statement interface {
	Node
	statementNode()
}

// Expression is a node that produces a value (e.g. a + b).
type Expression interface {
	Node
	expressionNode()
}

// Step is a top-level unit of a SAS program: a DATA step or a PROC step.
type Step interface {
	Node
	stepNode()
}

// Program is a whole SAS source file: an ordered list of steps.
type Program struct {
	Steps []Step
}

func (p *Program) String() string {
	var b strings.Builder
	for i, s := range p.Steps {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(s.String())
	}
	return b.String()
}

// DataStep is `data <names...>; <body> run;`. Datasets holds the one or more
// output dataset names declared after `data`. Body holds the statements.
type DataStep struct {
	Datasets []string
	Body     []Statement
}

func (d *DataStep) stepNode() {}

func (d *DataStep) String() string {
	var b strings.Builder
	b.WriteString("data " + strings.Join(d.Datasets, " ") + ";\n")
	for _, s := range d.Body {
		b.WriteString("  " + s.String() + "\n")
	}
	b.WriteString("run;")
	return b.String()
}

// ProcStep is `proc <name> <options>; <body> run|quit;`. For now options and
// body statements are kept generic; proc-specific structure is layered on in
// Phase 3.6 and the individual PROC implementations.
type ProcStep struct {
	Name    string       // procedure name, lowercased (e.g. "print", "sort")
	Data    string       // the data= dataset, if given ("" otherwise)
	Options []ProcOption // remaining options after proc <name> ... ;
	Body    []Statement  // statements between the proc statement and run/quit
	RawBody string       // verbatim source body, captured for free-form procs (PROC SQL)
}

// ProcOption is a single option on the PROC statement, e.g. `noobs` (flag) or
// `out=sorted` (Name="out", Value="sorted").
type ProcOption struct {
	Name  string
	Value string // "" for a bare flag option
}

func (p *ProcStep) stepNode() {}

func (p *ProcStep) String() string {
	var b strings.Builder
	b.WriteString("proc " + p.Name)
	if p.Data != "" {
		b.WriteString(" data=" + p.Data)
	}
	for _, o := range p.Options {
		if o.Value == "" {
			b.WriteString(" " + o.Name)
		} else {
			b.WriteString(" " + o.Name + "=" + o.Value)
		}
	}
	b.WriteString(";\n")
	for _, s := range p.Body {
		b.WriteString("  " + s.String() + "\n")
	}
	b.WriteString("run;")
	return b.String()
}

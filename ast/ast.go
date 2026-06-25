// Package ast defines the abstract syntax tree node types for SAS programs:
// the program and step containers, DATA step statements, PROC steps, and
// expression nodes.
package ast

import (
	"fmt"
	"strings"
)

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

// LibnameStatement is a global `libname <ref> <engine> "<connection>";` (or
// `libname <ref> clear;`). It binds a libref to an external library engine
// (e.g. a database) for the rest of the program. Engine is lowercased
// (postgres, sqlserver, oracle, db2, ...); Connection is the DSN/connection
// string. Clear is true for an unassign.
type LibnameStatement struct {
	Libref     string
	Engine     string
	Connection string
	Clear      bool
}

func (l *LibnameStatement) stepNode() {}
func (l *LibnameStatement) String() string {
	if l.Clear {
		return "libname " + l.Libref + " clear;"
	}
	return "libname " + l.Libref + " " + l.Engine + " \"" + l.Connection + "\";"
}

// TitleStatement is a global `title<n> "text";` (or a bare `title<n>;` that
// clears that line and all higher-numbered ones). Level is 1-10 (a bare `title`
// is level 1); Text is the title string (empty means clear). Titles persist
// across steps and are shown above procedure output.
type TitleStatement struct {
	Level int
	Text  string
}

func (t *TitleStatement) stepNode() {}
func (t *TitleStatement) String() string {
	if t.Text == "" {
		return fmt.Sprintf("title%d;", t.Level)
	}
	return fmt.Sprintf("title%d %q;", t.Level, t.Text)
}

// FootnoteStatement is a global `footnote<n> "text";` (or a bare `footnote<n>;`
// that clears that line and all higher-numbered ones), the bottom-of-output twin
// of TitleStatement. Footnotes persist across steps and are shown below
// procedure output.
type FootnoteStatement struct {
	Level int
	Text  string
}

func (f *FootnoteStatement) stepNode() {}
func (f *FootnoteStatement) String() string {
	if f.Text == "" {
		return fmt.Sprintf("footnote%d;", f.Level)
	}
	return fmt.Sprintf("footnote%d %q;", f.Level, f.Text)
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
	Datasets []string     // output dataset names, in order
	Outputs  []DatasetRef // output refs with optional dataset options (same order as Datasets)
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
	Name        string          // procedure name, lowercased (e.g. "print", "sort")
	Data        string          // the data= dataset, if given ("" otherwise)
	DataOptions *DatasetOptions // dataset options on data=ds(...), nil if none
	Options     []ProcOption    // remaining options after proc <name> ... ;
	Body        []Statement     // statements between the proc statement and run/quit
	RawBody     string          // verbatim source body, captured for free-form procs (PROC SQL)
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

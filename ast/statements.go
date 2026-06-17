package ast

import (
	"fmt"
	"strings"
)

// AssignmentStatement is `<name> = <expr>;`.
type AssignmentStatement struct {
	Name  string
	Value Expression
}

func (a *AssignmentStatement) statementNode() {}
func (a *AssignmentStatement) String() string {
	return a.Name + " = " + a.Value.String() + ";"
}

// SetStatement is `set <datasets...>;`.
type SetStatement struct {
	Datasets []string
}

func (s *SetStatement) statementNode() {}
func (s *SetStatement) String() string {
	return "set " + strings.Join(s.Datasets, " ") + ";"
}

// InputVar is one variable in an INPUT statement; Char is true if it was marked
// with `$` (character).
type InputVar struct {
	Name string
	Char bool
}

// InputStatement is `input <var [$]>...;`.
type InputStatement struct {
	Vars []InputVar
}

func (in *InputStatement) statementNode() {}
func (in *InputStatement) String() string {
	parts := make([]string, len(in.Vars))
	for i, v := range in.Vars {
		if v.Char {
			parts[i] = v.Name + " $"
		} else {
			parts[i] = v.Name
		}
	}
	return "input " + strings.Join(parts, " ") + ";"
}

// DatalinesStatement carries the raw inline data block, one element per line.
type DatalinesStatement struct {
	Lines []string
}

func (d *DatalinesStatement) statementNode() {}
func (d *DatalinesStatement) String() string {
	return "datalines; (" + fmt.Sprintf("%d", len(d.Lines)) + " lines)"
}

// IfStatement is `if <cond> then <stmt>; [else <stmt>;]`. Alternative is nil if
// there is no else branch.
type IfStatement struct {
	Condition   Expression
	Consequence Statement
	Alternative Statement
}

func (i *IfStatement) statementNode() {}
func (i *IfStatement) String() string {
	s := "if " + i.Condition.String() + " then " + i.Consequence.String()
	if i.Alternative != nil {
		s += " else " + i.Alternative.String()
	}
	return s
}

// SubsettingIf is a bare `if <cond>;` that drops the current row when the
// condition is false.
type SubsettingIf struct {
	Condition Expression
}

func (s *SubsettingIf) statementNode() {}
func (s *SubsettingIf) String() string {
	return "if " + s.Condition.String() + ";"
}

// DoKind distinguishes the forms of a DO statement.
type DoKind int

const (
	DoSimple    DoKind = iota // do; ... end;
	DoIterative               // do i = from to to [by by]; ... end;
	DoWhile                   // do while(cond); ... end;
	DoUntil                   // do until(cond); ... end;
)

// DoStatement is a DO ... END block. Fields used depend on Kind.
type DoStatement struct {
	Kind  DoKind
	Var   string     // iterative: loop variable
	From  Expression // iterative: start
	To    Expression // iterative: end
	By    Expression // iterative: step (nil => 1)
	Cond  Expression // while/until condition
	Body  []Statement
}

func (d *DoStatement) statementNode() {}
func (d *DoStatement) String() string {
	var b strings.Builder
	switch d.Kind {
	case DoIterative:
		b.WriteString("do " + d.Var + " = " + d.From.String() + " to " + d.To.String())
		if d.By != nil {
			b.WriteString(" by " + d.By.String())
		}
		b.WriteString(";")
	case DoWhile:
		b.WriteString("do while(" + d.Cond.String() + ");")
	case DoUntil:
		b.WriteString("do until(" + d.Cond.String() + ");")
	default:
		b.WriteString("do;")
	}
	for _, s := range d.Body {
		b.WriteString(" " + s.String())
	}
	b.WriteString(" end;")
	return b.String()
}

// OutputStatement is `output [datasets...];`. Datasets is empty for a bare
// `output;` (writes to all of the step's output datasets).
type OutputStatement struct {
	Datasets []string
}

func (o *OutputStatement) statementNode() {}
func (o *OutputStatement) String() string {
	if len(o.Datasets) == 0 {
		return "output;"
	}
	return "output " + strings.Join(o.Datasets, " ") + ";"
}

// KeepStatement is `keep <vars...>;`.
type KeepStatement struct {
	Vars []string
}

func (k *KeepStatement) statementNode() {}
func (k *KeepStatement) String() string {
	return "keep " + strings.Join(k.Vars, " ") + ";"
}

// DropStatement is `drop <vars...>;`.
type DropStatement struct {
	Vars []string
}

func (d *DropStatement) statementNode() {}
func (d *DropStatement) String() string {
	return "drop " + strings.Join(d.Vars, " ") + ";"
}

// ByStatement is `by [descending] <var> ...;` used in PROC SORT and BY-group
// processing. Descending[i] applies to Vars[i].
type ByStatement struct {
	Vars       []string
	Descending []bool
}

func (b *ByStatement) statementNode() {}
func (b *ByStatement) String() string {
	parts := make([]string, len(b.Vars))
	for i, v := range b.Vars {
		if i < len(b.Descending) && b.Descending[i] {
			parts[i] = "descending " + v
		} else {
			parts[i] = v
		}
	}
	return "by " + strings.Join(parts, " ") + ";"
}

// VarStatement is `var <vars...>;` (PROC PRINT column selection, etc.).
type VarStatement struct {
	Vars []string
}

func (v *VarStatement) statementNode() {}
func (v *VarStatement) String() string {
	return "var " + strings.Join(v.Vars, " ") + ";"
}

// RawStatement is a not-yet-structured statement, holding its source tokens'
// literals. It lets the parser keep moving over constructs that do not yet have
// dedicated nodes, without losing them. Phase 3+ replaces these incrementally.
type RawStatement struct {
	Text string
}

func (r *RawStatement) statementNode() {}
func (r *RawStatement) String() string { return r.Text + "; (raw)" }

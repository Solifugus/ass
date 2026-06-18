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
	return a.Name + " = " + str(a.Value) + ";"
}

// DatasetOptions are the options that may follow a dataset name in parentheses,
// e.g. `ds(keep=a b rename=(x=y) where=(a>0))`. Any combination may be present;
// an empty DatasetOptions (or nil) means no filtering. Rename maps a lowercased
// original variable name to its new name.
type DatasetOptions struct {
	Keep   []string
	Drop   []string
	Rename map[string]string
	Where  Expression
}

// IsEmpty reports whether the options impose no transformation.
func (o *DatasetOptions) IsEmpty() bool {
	return o == nil || (len(o.Keep) == 0 && len(o.Drop) == 0 && len(o.Rename) == 0 && o.Where == nil)
}

// SetStatement is `set <dataset[(options)] ...>;`.
type SetStatement struct {
	Refs []DatasetRef
}

func (s *SetStatement) statementNode() {}
func (s *SetStatement) String() string {
	parts := make([]string, len(s.Refs))
	for i, r := range s.Refs {
		parts[i] = r.Name
	}
	return "set " + strings.Join(parts, " ") + ";"
}

// DatasetRef is a dataset reference in a SET/MERGE/DATA statement, with optional
// dataset options. In is the `in=` flag variable name ("" if absent); Options is
// nil when no parenthesized options were given.
type DatasetRef struct {
	Name    string
	In      string
	Options *DatasetOptions
}

// MergeStatement is `merge ds1 [ds2 ...];`, match-merged by the step's BY
// variables. Each reference may carry an `in=` flag.
type MergeStatement struct {
	Refs []DatasetRef
}

func (m *MergeStatement) statementNode() {}
func (m *MergeStatement) String() string {
	parts := make([]string, len(m.Refs))
	for i, r := range m.Refs {
		parts[i] = r.Name
		if r.In != "" {
			parts[i] += "(in=" + r.In + ")"
		}
	}
	return "merge " + strings.Join(parts, " ") + ";"
}

// InputVar is one variable in an INPUT statement; Char is true if it was marked
// with `$` (character). Informat, when non-empty, is the informat spec used to
// read the field (e.g. "comma8.", "date9.", "$20.").
type InputVar struct {
	Name     string
	Char     bool
	Informat string
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
	s := "if " + str(i.Condition) + " then " + str(i.Consequence)
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
	return "if " + str(s.Condition) + ";"
}

// WhereStatement is `where <cond>;`. Unlike a subsetting IF, WHERE filters rows
// at read time and can reference only variables that exist in the input dataset
// (not variables computed in the step).
type WhereStatement struct {
	Condition Expression
}

func (w *WhereStatement) statementNode() {}
func (w *WhereStatement) String() string {
	return "where " + str(w.Condition) + ";"
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
		b.WriteString("do " + d.Var + " = " + str(d.From) + " to " + str(d.To))
		if d.By != nil {
			b.WriteString(" by " + d.By.String())
		}
		b.WriteString(";")
	case DoWhile:
		b.WriteString("do while(" + str(d.Cond) + ");")
	case DoUntil:
		b.WriteString("do until(" + str(d.Cond) + ");")
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

// RetainStatement is `retain <var [initial]>...;`. Retained variables keep their
// value across implicit-loop iterations instead of being reset to missing.
// Initials maps a (lowercased) variable name to its initial-value expression.
type RetainStatement struct {
	Vars     []string
	Initials map[string]Expression
}

func (r *RetainStatement) statementNode() {}
func (r *RetainStatement) String() string {
	return "retain " + strings.Join(r.Vars, " ") + ";"
}

// ArrayStatement is `array name{n} elem1 elem2 ...;`. Elements are the variable
// names the array indexes (1-based). Size is len(Elements) when declared `{*}`.
type ArrayStatement struct {
	Name     string
	Size     int
	Elements []string
}

func (a *ArrayStatement) statementNode() {}
func (a *ArrayStatement) String() string {
	return "array " + a.Name + "{" + str(&NumberLiteral{Literal: itoaArr(a.Size)}) + "} " + strings.Join(a.Elements, " ") + ";"
}

func itoaArr(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// ArrayElementAssignment is `name{index} = value;`.
type ArrayElementAssignment struct {
	Name  string
	Index Expression
	Value Expression
}

func (a *ArrayElementAssignment) statementNode() {}
func (a *ArrayElementAssignment) String() string {
	return a.Name + "{" + str(a.Index) + "} = " + str(a.Value) + ";"
}

// SumStatement is the SAS sum statement `<var> + <expr>;`, equivalent to
// `var = sum(var, expr)` with var retained and initialized to 0.
type SumStatement struct {
	Var  string
	Expr Expression
}

func (s *SumStatement) statementNode() {}
func (s *SumStatement) String() string {
	return s.Var + " + " + str(s.Expr) + ";"
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

// FormatStatement is `format <var-list> <format.> ...;` associating display
// formats with variables. Formats maps a (lowercased) variable name to its
// format specification (e.g. "dollar10.2").
type FormatStatement struct {
	Formats map[string]string
}

func (f *FormatStatement) statementNode() {}
func (f *FormatStatement) String() string {
	parts := make([]string, 0, len(f.Formats))
	for v, fm := range f.Formats {
		parts = append(parts, v+" "+fm)
	}
	return "format " + strings.Join(parts, " ") + ";"
}

// ModelStatement is `model <response> = <predictor...>;` for PROC REG/GLM.
type ModelStatement struct {
	Response   string
	Predictors []string
}

func (m *ModelStatement) statementNode() {}
func (m *ModelStatement) String() string {
	return "model " + m.Response + " = " + strings.Join(m.Predictors, " ") + ";"
}

// ValueRange is one `range = label` mapping within a VALUE statement. Low/High
// are the literal bounds as source text (numeric text or character content);
// for a single value High == Low. NoLow/NoHigh mark the `low`/`high` keywords;
// LowExcl/HighExcl mark exclusive bounds (`a <- b`, `a -< b`); Other is the
// catch-all `other=`.
type ValueRange struct {
	Low      string
	High     string
	NoLow    bool
	NoHigh   bool
	LowExcl  bool
	HighExcl bool
	Other    bool
	Label    string
}

// ValueStatement is `value [$]name <range>=<label> ...;` inside PROC FORMAT. It
// defines a user format; Char is true when the name began with `$` (a character
// format).
type ValueStatement struct {
	Name   string
	Char   bool
	Ranges []ValueRange
}

func (v *ValueStatement) statementNode() {}
func (v *ValueStatement) String() string {
	return "value " + v.Name + "; (" + fmt.Sprintf("%d", len(v.Ranges)) + " ranges)"
}

// ClassStatement is `class <vars...>;` — grouping variables for PROC MEANS/FREQ.
type ClassStatement struct {
	Vars []string
}

func (c *ClassStatement) statementNode() {}
func (c *ClassStatement) String() string {
	return "class " + strings.Join(c.Vars, " ") + ";"
}

// TablesStatement is `tables <request...>;` — the tables PROC FREQ produces.
// Each request is one or more variables crossed with `*`: a one-element request
// is a one-way frequency table, a two-element request is a two-way
// cross-tabulation. Vars is the flattened variable list (kept for callers that
// only need one-way names).
type TablesStatement struct {
	Vars     []string
	Requests [][]string
}

func (t *TablesStatement) statementNode() {}
func (t *TablesStatement) String() string {
	parts := make([]string, len(t.Requests))
	for i, req := range t.Requests {
		parts[i] = strings.Join(req, "*")
	}
	return "tables " + strings.Join(parts, " ") + ";"
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

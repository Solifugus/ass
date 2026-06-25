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
	// FirstObs/Obs bound the observation range read from the data set, by
	// position (1-based, 0 = unset). FirstObs selects the first observation; Obs
	// is the number of the last observation processed (SAS semantics — a count of
	// the highest observation, not a row count).
	FirstObs int
	Obs      int
}

// IsEmpty reports whether the options impose no transformation.
func (o *DatasetOptions) IsEmpty() bool {
	return o == nil || (len(o.Keep) == 0 && len(o.Drop) == 0 && len(o.Rename) == 0 &&
		o.Where == nil && o.FirstObs == 0 && o.Obs == 0)
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
//
// Column/pointer input adds explicit positioning. ColStart>0 marks column input:
// the value is read from the 1-based inclusive column range ColStart..ColEnd
// (ColEnd==0 means a single column at ColStart). At>0 is a `@n` absolute column
// pointer set before reading this variable; Plus>0 is a `+n` relative skip. Any
// of these (or a formatted read that follows a pointer) puts the INPUT statement
// in column/pointer mode rather than delimited list mode.
// Line>0 is a `#n` line pointer set before reading this variable: the value is
// read from the n-th physical line of the current logical record (and the column
// pointer resets to 1). A line pointer makes the INPUT statement read across
// multiple physical lines per observation.
type InputVar struct {
	Name     string
	Char     bool
	Informat string
	ColStart int
	ColEnd   int
	At       int
	Plus     int
	Line     int
}

// InputStatement is `input <var [$]>...;`. TrailingAt records a line-hold
// modifier at the end of the statement: 0 = none (the record is released after
// this INPUT), 1 = a single `@` (hold the line for the next INPUT in the same
// iteration), 2 = `@@` (hold the line across iterations, so several observations
// can be read from one physical line).
type InputStatement struct {
	Vars       []InputVar
	TrailingAt int
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

// InfileStatement is `infile "<path>" <options>;` — it names an external flat
// file as the record source for subsequent INPUT statements (in place of inline
// DATALINES). Delimiter is the field delimiter (empty = whitespace list input);
// DSD enables CSV-style parsing (quoted fields, consecutive delimiters yield
// missing). Firstobs/Obs (1-based, 0 = unset) bound the line range.
type InfileStatement struct {
	Path      string
	Delimiter string
	DSD       bool
	Firstobs  int
	Obs       int
	Missover  bool
	Truncover bool
}

func (in *InfileStatement) statementNode() {}
func (in *InfileStatement) String() string {
	return "infile \"" + in.Path + "\";"
}

// FileStatement is `file "<path>" <options>;` — it names an external flat file
// as the destination for subsequent PUT statements. Delimiter is the field
// separator for list output (empty = single blank, or comma under DSD); DSD
// enables CSV-style writing (quote values containing the delimiter or a quote).
type FileStatement struct {
	Path      string
	Delimiter string
	DSD       bool
}

func (f *FileStatement) statementNode() {}
func (f *FileStatement) String() string { return "file \"" + f.Path + "\";" }

// PutItem is one element of a PUT statement: either a quoted string literal
// (IsLiteral) or a variable reference. Format, when non-empty, is the inline
// format spec written after the variable (e.g. "dollar8.2", "$10").
//
// Column/pointer output adds explicit positioning, mirroring INPUT: ColStart>0
// writes the value into the 1-based inclusive range ColStart..ColEnd (character
// left-justified, numeric right-justified within the width); At>0 is a `@n`
// absolute column pointer set before writing this item; Plus>0 is a `+n` relative
// skip. Any of these put the PUT statement in column/pointer mode rather than
// delimiter-joined list mode.
// Line>0 is a `#n` line pointer set before this item: the item (and subsequent
// items until the next `#n`) is written to the n-th physical line of the output
// record, so one PUT statement can emit several lines.
type PutItem struct {
	IsLiteral bool
	Literal   string
	Var       string
	Format    string
	ColStart  int
	ColEnd    int
	At        int
	Plus      int
	Line      int
	// Named renders the item as `name=value` (SAS named output, `put x=;`).
	// AllVars is the `_all_` pseudo-item: every PDV variable written as
	// `name=value`, expanded at render time.
	Named   bool
	AllVars bool
}

// PutStatement is `put <item>...;`, writing the items as one line to the current
// FILE destination (or the log if none). Items are joined by the file's
// delimiter (a single blank for default list output). TrailingAt records a
// trailing line-hold modifier: 1 for `@` (hold the output line within the
// iteration, released automatically at the iteration boundary) and 2 for `@@`
// (hold the output line across iterations, released only by a PUT without a
// trailing hold or at end of step); 0 means none.
type PutStatement struct {
	Items      []PutItem
	TrailingAt int
}

func (p *PutStatement) statementNode() {}
func (p *PutStatement) String() string {
	parts := make([]string, len(p.Items))
	for i, it := range p.Items {
		if it.IsLiteral {
			parts[i] = "\"" + it.Literal + "\""
		} else {
			parts[i] = it.Var
		}
	}
	return "put " + strings.Join(parts, " ") + ";"
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
	Kind DoKind
	Var  string     // iterative: loop variable
	From Expression // iterative: start
	To   Expression // iterative: end
	By   Expression // iterative: step (nil => 1)
	Cond Expression // while/until condition
	Body []Statement
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

// RenameStatement is `rename old=new old2=new2 ...;` renaming variables in the
// step's output dataset(s). Map keys are lowercased old names; values are the new
// names (case preserved). Within the step the original names remain in use; the
// rename is applied when the output columns are built.
type RenameStatement struct {
	Map map[string]string
}

func (r *RenameStatement) statementNode() {}
func (r *RenameStatement) String() string {
	parts := make([]string, 0, len(r.Map))
	for o, n := range r.Map {
		parts = append(parts, o+"="+n)
	}
	return "rename " + strings.Join(parts, " ") + ";"
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

// LabelStatement is `label <var> = "text" ...;` associating descriptive labels
// with variables. Labels maps a (lowercased) variable name to its label text.
type LabelStatement struct {
	Labels map[string]string
}

func (l *LabelStatement) statementNode() {}
func (l *LabelStatement) String() string {
	parts := make([]string, 0, len(l.Labels))
	for v, txt := range l.Labels {
		parts = append(parts, v+`="`+txt+`"`)
	}
	return "label " + strings.Join(parts, " ") + ";"
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
// format). Invalue is true when the statement was `invalue` (a user informat:
// the ranges match the input being read and the labels are the result values).
type ValueStatement struct {
	Name    string
	Char    bool
	Invalue bool
	Ranges  []ValueRange
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
	// Options holds the `/ options` tail (lowercased), e.g. nocol/norow/
	// nopercent/nocum/nofreq/list/chisq.
	Options []string
}

// HasOption reports whether the tables `/ options` tail included opt.
func (t *TablesStatement) HasOption(opt string) bool {
	for _, o := range t.Options {
		if o == opt {
			return true
		}
	}
	return false
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

// ProofStatement is one assertion in a PROC PROOF step body. Kind selects the
// check ("require", "type", "notnull", "values", "range", "rule", "unique",
// "key"); the remaining fields are populated as that kind requires. The optional
// per-assertion tail (`/ severity= message=`) fills Severity and Message.
type ProofStatement struct {
	Kind     string     // assertion kind (see above)
	Vars     []string   // columns: require/notnull/unique vars; target column is Vars[0] for values/range; child key columns for key; declared columns for type
	Values   []string   // allowed-set literals for `values <var> in (...)`; declared types (parallel to Vars) for `type`
	Low      string     // inclusive lower bound for `range <var> lo - hi` (literal text)
	High     string     // inclusive upper bound for `range`
	Op       string     // comparison operator for the relational `range <var> <op> <num>` form (e.g. ">=")
	Bound    string     // bound literal for the relational range form
	Expr     Expression // boolean expression for `rule`
	Label    string     // label for `rule "label": ...`
	RefTable string     // parent table for `key <cols> references <table>(<cols>)`
	RefCols  []string   // parent columns for `key` (parallel to Vars)
	Severity string     // "warn"/"error"/"" (empty = step default)
	Message  string     // custom violation message ("" = a generated default)
}

func (s *ProofStatement) statementNode() {}
func (s *ProofStatement) String() string {
	b := strings.Builder{}
	b.WriteString(s.Kind)
	if s.Label != "" {
		b.WriteString(" \"" + s.Label + "\"")
	}
	if len(s.Vars) > 0 {
		b.WriteString(" " + strings.Join(s.Vars, " "))
	}
	if s.Kind == "values" && len(s.Values) > 0 {
		b.WriteString(" in (" + strings.Join(s.Values, " ") + ")")
	}
	if s.Kind == "range" {
		if s.Op != "" {
			b.WriteString(" " + s.Op + " " + s.Bound)
		} else {
			b.WriteString(" " + s.Low + " - " + s.High)
		}
	}
	if s.Kind == "type" {
		b.Reset()
		b.WriteString("type")
		for i, v := range s.Vars {
			t := ""
			if i < len(s.Values) {
				t = s.Values[i]
			}
			b.WriteString(" " + v + "=" + t)
		}
	}
	if s.Kind == "key" {
		b.WriteString(" references " + s.RefTable + "(" + strings.Join(s.RefCols, " ") + ")")
	}
	if s.Expr != nil {
		b.WriteString(": " + s.Expr.String())
	}
	if s.Severity != "" {
		b.WriteString(" / severity=" + s.Severity)
	}
	return b.String() + ";"
}

// RawStatement is a not-yet-structured statement, holding its source tokens'
// literals. It lets the parser keep moving over constructs that do not yet have
// dedicated nodes, without losing them. Phase 3+ replaces these incrementally.
type RawStatement struct {
	Text string
}

func (r *RawStatement) statementNode() {}
func (r *RawStatement) String() string { return r.Text + "; (raw)" }

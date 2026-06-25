package runtime

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

// PROC PROOF — a data-quality validation step. It is a read-only pass over a
// dataset that checks declared assertions and emits a verdict (a listing, an
// optional out= violations dataset, and a non-zero exit when an error-level
// assertion fails) instead of producing a transformed dataset. See
// docs/proofing.md for the design. PROC PROOF lives in the runtime package (not
// proc/) because it reuses the DATA-step expression evaluator and PDV.

// proofResult accumulates one assertion's outcome over the row scan.
type proofResult struct {
	stmt     *ast.ProofStatement
	desc     string // human-readable description for the report
	ruleKey  string // value written to out=._rule_ (label for rule, else desc)
	severity string // resolved severity: "warn" or "error"
	checked  int    // rows evaluated (0 if it could not run)
	violObs  []int  // 1-based observation numbers that violated, in scan order
	couldRun bool
	why      string // when !couldRun, the reason (e.g. missing column)
}

func (r *proofResult) failed() bool { return r.couldRun && len(r.violObs) > 0 }

// runProof executes a PROC PROOF step.
func runProof(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	ds, ok := lib.Get(step.Data)
	if !ok {
		logger.Error("PROC PROOF: data set %s not found.", strings.ToUpper(step.Data))
		return nil
	}

	outName, maxSample, stepSeverity := parseProofOptions(step)

	var results []*proofResult
	for _, s := range step.Body {
		ps, isProof := s.(*ast.ProofStatement)
		if !isProof {
			continue
		}
		results = append(results, &proofResult{
			stmt:     ps,
			desc:     proofDesc(ps),
			ruleKey:  proofRuleKey(ps),
			severity: resolveSeverity(ps.Severity, stepSeverity),
			couldRun: true,
		})
	}

	// Schema check: any assertion referencing a column the dataset lacks cannot
	// run. `require` fails when a listed column is missing; `type` fails when a
	// column's kind does not match the declared type. Both are dataset-level.
	for _, r := range results {
		switch r.stmt.Kind {
		case "require":
			for _, v := range r.stmt.Vars {
				if !ds.HasColumn(v) {
					r.violObs = append(r.violObs, 0) // dataset-level violation
					r.why = "missing column: " + v
				}
			}
			r.checked = 1
		case "type":
			r.checked = len(r.stmt.Vars)
			for i, v := range r.stmt.Vars {
				k, ok := columnKind(ds, v)
				if !ok {
					r.couldRun = false
					r.why = "references unknown column " + v
					break
				}
				want, known := declaredKind(r.stmt.Values[i])
				if known && k != want {
					r.violObs = append(r.violObs, 0)
				}
			}
		default:
			for _, v := range r.stmt.Vars {
				if !ds.HasColumn(v) {
					r.couldRun = false
					r.why = "references unknown column " + v
					break
				}
			}
		}
	}

	// One linear scan: build a PDV per row and evaluate every row-local assertion.
	// `unique` accumulates key→obs across the scan and resolves afterward.
	pdv := NewPDV()
	for _, c := range ds.Columns {
		pdv.Declare(c.Name, c.Kind)
	}
	uniqueKeys := make([]map[string][]int, len(results))
	keySets := make([]map[string]bool, len(results))
	for i, r := range results {
		if !r.couldRun {
			continue
		}
		switch r.stmt.Kind {
		case "unique":
			uniqueKeys[i] = make(map[string][]int)
		case "key":
			set, err := loadKeySet(lib, r.stmt.RefTable, r.stmt.RefCols)
			if err != nil {
				r.couldRun = false
				r.why = err.Error()
				continue
			}
			keySets[i] = set
		}
	}

	for ri, row := range ds.Rows {
		obs := ri + 1
		for _, c := range ds.Columns {
			pdv.Set(c.Name, ds.Get(row, c.Name))
		}
		for i, r := range results {
			if !r.couldRun || r.stmt.Kind == "require" || r.stmt.Kind == "type" {
				continue
			}
			switch r.stmt.Kind {
			case "unique":
				key := uniqueKey(pdv, r.stmt.Vars)
				uniqueKeys[i][key] = append(uniqueKeys[i][key], obs)
			case "key":
				r.checked++
				if k, ok := foreignKey(pdv, r.stmt.Vars); ok && !keySets[i][k] { // any-missing FK passes
					r.violObs = append(r.violObs, obs)
				}
			default:
				r.checked++
				if proofRowViolates(r.stmt, pdv, logger) {
					r.violObs = append(r.violObs, obs)
				}
			}
		}
	}

	// Resolve set-level assertions: every row whose key is duplicated violates.
	for i, r := range results {
		if !r.couldRun || r.stmt.Kind != "unique" {
			continue
		}
		r.checked = len(ds.Rows)
		var dup []int
		for _, obsList := range uniqueKeys[i] {
			if len(obsList) > 1 {
				dup = append(dup, obsList...)
			}
		}
		sort.Ints(dup)
		r.violObs = dup
	}

	writeProofReport(ds, results, maxSample, logger)
	if outName != "" {
		if err := writeProofViolations(lib, ds, results, outName, logger); err != nil {
			return err
		}
	}
	return finalizeProof(ds, results, logger)
}

// parseProofOptions reads the PROC PROOF step options: out=, maxsample=,
// severity=.
func parseProofOptions(step *ast.ProcStep) (outName string, maxSample int, stepSeverity string) {
	maxSample = 20
	for _, o := range step.Options {
		switch strings.ToLower(o.Name) {
		case "out":
			outName = o.Value
		case "maxsample":
			if n, err := strconv.Atoi(o.Value); err == nil && n >= 0 {
				maxSample = n
			}
		case "severity":
			stepSeverity = strings.ToLower(o.Value)
		}
	}
	return outName, maxSample, stepSeverity
}

func resolveSeverity(assertion, step string) string {
	if assertion == "warn" || assertion == "error" {
		return assertion
	}
	if step == "warn" || step == "error" {
		return step
	}
	return "error"
}

// proofRowViolates reports whether a single row violates a row-local assertion.
func proofRowViolates(ps *ast.ProofStatement, pdv *PDV, logger *log.Logger) bool {
	switch ps.Kind {
	case "notnull":
		for _, v := range ps.Vars {
			if pdv.Get(v).IsMissing() {
				return true
			}
		}
		return false
	case "values":
		if len(ps.Vars) == 0 {
			return false
		}
		return !valueInSet(pdv.Get(ps.Vars[0]), ps.Values)
	case "range":
		if len(ps.Vars) == 0 {
			return false
		}
		val := pdv.Get(ps.Vars[0])
		if val.IsMissing() {
			return true
		}
		if ps.Op != "" { // relational form: range x >= 0
			bound, ok := parseFloat(ps.Bound)
			return ok && !numCmp(val.Num, ps.Op, bound)
		}
		lo, loOK := parseFloat(ps.Low)
		hi, hiOK := parseFloat(ps.High)
		if loOK && val.Num < lo {
			return true
		}
		if hiOK && val.Num > hi {
			return true
		}
		return false
	case "rule":
		if ps.Expr == nil {
			return false
		}
		v, err := Eval(ps.Expr, pdv)
		if err != nil {
			logger.Warning("PROC PROOF: rule %q: %v", ps.Label, err)
			return true
		}
		return v.IsMissing() || v.Num == 0 // SAS falsity: missing or zero
	}
	return false
}

// valueInSet reports whether v matches any allowed literal. Numeric values
// compare numerically; character values compare by string. A missing value is
// never in the set.
func valueInSet(v table.Value, allowed []string) bool {
	if v.IsMissing() {
		return false
	}
	for _, a := range allowed {
		if v.Kind == table.Numeric {
			if f, ok := parseFloat(a); ok && f == v.Num {
				return true
			}
		} else if v.Str == a {
			return true
		}
	}
	return false
}

// uniqueKey builds a stable key from the listed columns of the current row.
func uniqueKey(pdv *PDV, vars []string) string {
	parts := make([]string, len(vars))
	for i, v := range vars {
		parts[i] = scalarKey(pdv.Get(v))
	}
	return strings.Join(parts, "\x01")
}

// scalarKey encodes a single value as a comparable string. A missing value maps
// to a sentinel distinct from any real value.
func scalarKey(v table.Value) string {
	if v.IsMissing() {
		return "\x00"
	}
	if v.Kind == table.Numeric {
		return strconv.FormatFloat(v.Num, 'g', -1, 64)
	}
	return v.Str
}

// loadKeySet loads a parent table's key column(s) into a set, for
// `key … references` referential-integrity checks. The parent is resolved via the
// library (WORK, a base/directory libref, or an external database libref). A
// parent row with any missing key component is skipped (it can never match a
// non-missing foreign key).
func loadKeySet(lib *table.Library, refTable string, refCols []string) (map[string]bool, error) {
	if refTable == "" || len(refCols) == 0 {
		return nil, fmt.Errorf("malformed key…references")
	}
	parent, ok, err := lib.Resolve(refTable)
	if err != nil {
		return nil, fmt.Errorf("parent table %s: %v", strings.ToUpper(refTable), err)
	}
	if !ok {
		return nil, fmt.Errorf("parent table %s not found", strings.ToUpper(refTable))
	}
	for _, c := range refCols {
		if !parent.HasColumn(c) {
			return nil, fmt.Errorf("parent table %s has no column %s", strings.ToUpper(refTable), c)
		}
	}
	set := make(map[string]bool, parent.NObs())
	for _, row := range parent.Rows {
		parts := make([]string, len(refCols))
		missing := false
		for i, c := range refCols {
			v := parent.Get(row, c)
			if v.IsMissing() {
				missing = true
				break
			}
			parts[i] = scalarKey(v)
		}
		if !missing {
			set[strings.Join(parts, "\x01")] = true
		}
	}
	return set, nil
}

// foreignKey builds the composite key string for the current row from the child
// key columns. ok is false when any component is missing — a foreign key with a
// missing part passes (SQL NULL-FK semantics) and is not checked for membership.
func foreignKey(pdv *PDV, vars []string) (key string, ok bool) {
	parts := make([]string, len(vars))
	for i, v := range vars {
		val := pdv.Get(v)
		if val.IsMissing() {
			return "", false
		}
		parts[i] = scalarKey(val)
	}
	return strings.Join(parts, "\x01"), true
}

// numCmp evaluates `a <op> b` for the relational range form.
func numCmp(a float64, op string, b float64) bool {
	switch op {
	case ">=":
		return a >= b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case "<":
		return a < b
	case "=":
		return a == b
	case "^=":
		return a != b
	}
	return false
}

// columnKind returns the declared kind of a dataset column.
func columnKind(ds *table.Dataset, name string) (table.Kind, bool) {
	ln := strings.ToLower(name)
	for _, c := range ds.Columns {
		if strings.ToLower(c.Name) == ln {
			return c.Kind, true
		}
	}
	return table.Numeric, false
}

// declaredKind maps a `type` declaration ("num"/"numeric", "char"/"character")
// to a table.Kind. The second return is false for an unrecognized declaration
// (which is then not checked).
func declaredKind(s string) (table.Kind, bool) {
	switch strings.ToLower(s) {
	case "num", "numeric", "n":
		return table.Numeric, true
	case "char", "character", "c", "$":
		return table.Character, true
	}
	return table.Numeric, false
}

func parseFloat(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

// writeProofReport prints the per-assertion listing to stdout.
func writeProofReport(ds *table.Dataset, results []*proofResult, maxSample int, logger *log.Logger) {
	var b strings.Builder
	fmt.Fprintf(&b, "\nPROC PROOF — %s.%s (%d obs)\n\n",
		strings.ToUpper(ds.Lib), strings.ToUpper(ds.Name), ds.NObs())
	fmt.Fprintf(&b, "  %-40s %-6s %s\n", "Assertion", "Result", "Violations/Checked")
	for _, r := range results {
		result := "PASS"
		switch {
		case !r.couldRun:
			result = "N/RUN"
		case r.failed():
			result = "FAIL"
		}
		fmt.Fprintf(&b, "  %-40s %-6s %d/%d\n", truncate(r.desc, 40), result, len(r.violObs), r.checked)
		if !r.couldRun {
			fmt.Fprintf(&b, "      (could not run: %s)\n", r.why)
			continue
		}
		if r.failed() && maxSample > 0 {
			sample := r.violObs
			if len(sample) > maxSample {
				sample = sample[:maxSample]
			}
			fmt.Fprintf(&b, "      offending obs: %s\n", joinInts(sample))
		}
	}
	fmt.Fprint(logger.Listing(), b.String())
}

// writeProofViolations builds and stores the out= dataset: one row per
// (source row × failed assertion), annotated with _rule_ and _obs_.
func writeProofViolations(lib *table.Library, ds *table.Dataset, results []*proofResult, outName string, logger *log.Logger) error {
	out := table.NewDataset("", outName)
	for _, c := range ds.Columns {
		out.AddColumn(c)
	}
	out.AddColumn(table.Column{Name: "_rule_", Kind: table.Character})
	out.AddColumn(table.Column{Name: "_obs_", Kind: table.Numeric})

	type viol struct {
		obs int
		key string
	}
	var viols []viol
	for _, r := range results {
		for _, obs := range r.violObs {
			if obs == 0 {
				continue // dataset-level (require) violation has no source row
			}
			viols = append(viols, viol{obs: obs, key: r.ruleKey})
		}
	}
	sort.SliceStable(viols, func(i, j int) bool {
		if viols[i].obs != viols[j].obs {
			return viols[i].obs < viols[j].obs
		}
		return viols[i].key < viols[j].key
	})

	for _, v := range viols {
		src := ds.Rows[v.obs-1]
		row := table.Row{}
		for _, c := range ds.Columns {
			row[strings.ToLower(c.Name)] = ds.Get(src, c.Name)
		}
		row["_rule_"] = table.Char(v.key)
		row["_obs_"] = table.Num(float64(v.obs))
		out.AppendRow(row)
	}

	if err := lib.Store(outName, out); err != nil {
		logger.Error("PROC PROOF: cannot write out=%s: %v", strings.ToUpper(outName), err)
		return nil
	}
	logger.DatasetNote(out.Lib, out.Name, out.NObs(), len(out.Columns))
	return nil
}

// finalizeProof emits the verdict NOTE/WARNING/ERROR lines. An error-level
// failure logs ERROR (which sets a non-zero process exit) but does not halt the
// program; warn-level failures log WARNING and do not affect the exit code.
func finalizeProof(ds *table.Dataset, results []*proofResult, logger *log.Logger) error {
	var errFails, warnFails int
	for _, r := range results {
		if !r.failed() {
			continue
		}
		if r.severity == "warn" {
			warnFails++
			logger.Warning("PROC PROOF: assertion failed (%s): %s — %d/%d rows.",
				r.severity, proofMessage(r), len(r.violObs), r.checked)
		} else {
			errFails++
			logger.Error("PROC PROOF: assertion failed (%s): %s — %d/%d rows.",
				r.severity, proofMessage(r), len(r.violObs), r.checked)
		}
	}
	switch {
	case errFails > 0:
		logger.Error("PROC PROOF: %d error-level assertion(s) failed on %s.%s.",
			errFails, strings.ToUpper(ds.Lib), strings.ToUpper(ds.Name))
	case warnFails > 0:
		logger.Note("PROC PROOF: %d warn-level assertion(s) failed; data accepted.", warnFails)
	default:
		logger.Note("PROC PROOF: all assertions passed on %s.%s.",
			strings.ToUpper(ds.Lib), strings.ToUpper(ds.Name))
	}
	return nil
}

// proofMessage is the user-facing message for a failed assertion: the custom
// message if given, else the assertion description.
func proofMessage(r *proofResult) string {
	if r.stmt.Message != "" {
		return r.stmt.Message
	}
	return r.desc
}

// proofDesc renders a one-line description of an assertion for the report.
func proofDesc(ps *ast.ProofStatement) string {
	switch ps.Kind {
	case "values":
		return "values " + first(ps.Vars) + " in (" + strings.Join(ps.Values, " ") + ")"
	case "range":
		if ps.Op != "" {
			return "range " + first(ps.Vars) + " " + ps.Op + " " + ps.Bound
		}
		return "range " + first(ps.Vars) + " " + ps.Low + " - " + ps.High
	case "rule":
		if ps.Label != "" {
			return "rule \"" + ps.Label + "\""
		}
		if ps.Expr != nil {
			return "rule " + ps.Expr.String()
		}
		return "rule"
	case "type":
		pairs := make([]string, len(ps.Vars))
		for i, v := range ps.Vars {
			t := ""
			if i < len(ps.Values) {
				t = ps.Values[i]
			}
			pairs[i] = v + "=" + t
		}
		return "type " + strings.Join(pairs, " ")
	case "key":
		return "key " + strings.Join(ps.Vars, " ") + " references " + ps.RefTable + "(" + strings.Join(ps.RefCols, " ") + ")"
	default: // require, notnull, unique
		return ps.Kind + " " + strings.Join(ps.Vars, " ")
	}
}

// proofRuleKey is the value stored in out=._rule_ for an assertion: the rule's
// label when present (so violations filter cleanly by label), else its
// description.
func proofRuleKey(ps *ast.ProofStatement) string {
	if ps.Kind == "rule" && ps.Label != "" {
		return ps.Label
	}
	return proofDesc(ps)
}

func first(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func joinInts(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, " ")
}

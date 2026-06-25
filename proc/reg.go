package proc

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() {
	Register("reg", regProc{})
	Register("glm", regProc{}) // GLM with continuous predictors reduces to OLS
}

// regProc implements PROC REG / PROC GLM: ordinary least-squares linear
// regression for `model y = x1 x2 ...;`, with categorical predictors named in a
// `class` statement expanded into indicator variables. It reports the parameter
// estimates (Estimate, StdErr, tValue, Pr>|t|) and the model R-square.
//
// CLASS effects use REFERENCE-CELL coding: a class variable with k levels
// contributes k-1 indicators, with its last (highest-sorted) level as the
// reference (estimate fixed at 0). This is numerically correct for the fit and
// for level-vs-reference differences, but is INTENTIONALLY NOT identical to SAS
// GLM's singular/generalized-inverse parameterization (which keeps all levels
// and flags the aliased one "Biased"). The intercept and per-level estimates
// will therefore differ from SAS even though predictions and contrasts agree.
// See the design→solve→render split: swapping reference coding for SAS's sweep
// is localized to buildDesign + olsSolve.
type regProc struct{}

func (regProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	src, ok := lib.Get(step.Data)
	if !ok {
		logger.Error("PROC REG: data set %s not found.", strings.ToUpper(step.Data))
		return nil
	}
	var model *ast.ModelStatement
	var classVars []string
	for _, s := range step.Body {
		switch st := s.(type) {
		case *ast.ModelStatement:
			if model == nil {
				model = st
			}
		case *ast.ClassStatement:
			classVars = append(classVars, st.Vars...)
		}
	}
	if model == nil {
		logger.Error("PROC REG: a MODEL statement is required.")
		return nil
	}

	fit, dsg, err := fitModel(src, model.Response, model.Predictors, classVars)
	if err != nil {
		logger.Error("PROC REG: %v", err)
		return nil
	}

	result := table.NewDataset("", "_reg_")
	result.AddColumn(table.Column{Name: "Parameter", Kind: table.Character})
	result.AddColumn(table.Column{Name: "DF", Kind: table.Numeric})
	result.AddColumn(table.Column{Name: "Estimate", Kind: table.Numeric, Format: "12.5"})
	result.AddColumn(table.Column{Name: "StdErr", Kind: table.Numeric, Format: "12.5"})
	result.AddColumn(table.Column{Name: "tValue", Kind: table.Numeric, Format: "8.2"})
	result.AddColumn(table.Column{Name: "Pr>|t|", Kind: table.Numeric, Format: "7.4"})

	hasRef := false
	for _, prm := range dsg.params {
		if prm.reference {
			hasRef = true
			result.AppendRow(table.Row{
				"parameter": table.Char(prm.label + " (ref)"),
				"df":        table.Num(0),
				"estimate":  table.Num(0),
				"stderr":    table.MissingNum(),
				"tvalue":    table.MissingNum(),
				"pr>|t|":    table.MissingNum(),
			})
			continue
		}
		j := prm.col
		result.AppendRow(table.Row{
			"parameter": table.Char(prm.label),
			"df":        table.Num(1),
			"estimate":  table.Num(fit.beta[j]),
			"stderr":    numOrMissing(fit.stderr[j]),
			"tvalue":    numOrMissing(fit.tvalue[j]),
			"pr>|t|":    numOrMissing(fit.pvalue[j]),
		})
	}

	out := logger.Listing()
	fmt.Fprintf(out, "Dependent Variable: %s\n", model.Response)
	fmt.Fprintf(out, "R-Square: %.5f   Observations: %d\n\n", fit.rSquare, fit.n)
	emitListing(logger, result, printOptions{})
	if hasRef {
		fmt.Fprintln(out, "\nNOTE: (ref) marks a class variable's reference level (estimate fixed at 0;")
		fmt.Fprintln(out, "      reference-cell coding, not SAS GLM's generalized-inverse parameterization).")
	}
	return nil
}

// olsFit holds a fitted model's solved coefficients and diagnostics. beta and
// the diagnostic slices are indexed by design-matrix column.
type olsFit struct {
	beta    []float64
	stderr  []float64
	tvalue  []float64
	pvalue  []float64 // two-sided Pr > |t|
	rSquare float64
	n       int
	dfe     int
}

// designParam is one row of the parameter-estimates table. col is the index of
// the parameter's column in the design matrix; a reference level is not an
// estimated column (col is unused, reference is true, estimate is fixed at 0).
type designParam struct {
	label     string
	col       int
	reference bool
}

// design is a built model matrix plus the parameter rows to display. ncols is
// the number of estimated columns (intercept + continuous + non-reference
// indicators).
type design struct {
	X      [][]float64
	y      []float64
	params []designParam
	ncols  int
}

// ols is the no-CLASS entry point retained for callers/tests that only need the
// fit (beta indexed as [intercept, predictors...]).
func ols(ds *table.Dataset, response string, predictors []string) (olsFit, error) {
	fit, _, err := fitModel(ds, response, predictors, nil)
	return fit, err
}

// fitModel builds the design matrix (expanding CLASS variables) and solves it.
// The data→design step and the linear-algebra solve are separated so that
// replacing reference-cell coding with SAS's sweep/generalized-inverse later is
// a localized change.
func fitModel(ds *table.Dataset, response string, predictors, classVars []string) (olsFit, *design, error) {
	dsg, err := buildDesign(ds, response, predictors, classVars)
	if err != nil {
		return olsFit{}, nil, err
	}
	fit, err := olsSolve(dsg.X, dsg.y, dsg.ncols)
	if err != nil {
		return olsFit{}, nil, err
	}
	return fit, dsg, nil
}

// buildDesign constructs the design matrix and the display parameters. Only
// complete cases (no missing response/continuous/class value) are used. CLASS
// variables expand to reference-cell indicators (last sorted level = reference).
func buildDesign(ds *table.Dataset, response string, predictors, classVars []string) (*design, error) {
	isClass := map[string]bool{}
	for _, c := range classVars {
		isClass[strings.ToLower(c)] = true
	}

	// 1. Complete-case rows: response and every predictor present.
	var rows []table.Row
	for _, r := range ds.Rows {
		if ds.Get(r, response).IsMissing() {
			continue
		}
		ok := true
		for _, p := range predictors {
			if ds.Get(r, p).IsMissing() {
				ok = false
				break
			}
		}
		if ok {
			rows = append(rows, r)
		}
	}

	// 2. For each class predictor, its sorted distinct levels (over complete cases).
	levels := map[string][]string{}
	for _, p := range predictors {
		if isClass[strings.ToLower(p)] {
			levels[p] = classLevels(ds, rows, p)
		}
	}

	// 3. Assign columns and display parameters in MODEL order.
	dsg := &design{}
	dsg.params = append(dsg.params, designParam{label: "Intercept", col: 0})
	ncol := 1 // intercept is column 0
	type colSpec struct {
		predictor string
		level     string // "" => continuous
	}
	specs := []colSpec{{}} // column 0 = intercept (sentinel)
	for _, p := range predictors {
		if !isClass[strings.ToLower(p)] {
			dsg.params = append(dsg.params, designParam{label: p, col: ncol})
			specs = append(specs, colSpec{predictor: p})
			ncol++
			continue
		}
		lv := levels[p]
		for i, l := range lv {
			label := fmt.Sprintf("%s %s", p, l)
			if i == len(lv)-1 { // last level is the reference
				dsg.params = append(dsg.params, designParam{label: label, reference: true})
				continue
			}
			dsg.params = append(dsg.params, designParam{label: label, col: ncol})
			specs = append(specs, colSpec{predictor: p, level: l})
			ncol++
		}
	}
	dsg.ncols = ncol

	n := len(rows)
	if n <= ncol {
		return nil, fmt.Errorf("not enough complete observations (%d) for %d parameters", n, ncol)
	}

	// 4. Materialize X and y.
	dsg.y = make([]float64, n)
	dsg.X = make([][]float64, n)
	for k, r := range rows {
		dsg.y[k] = ds.Get(r, response).Num
		row := make([]float64, ncol)
		row[0] = 1 // intercept
		for c := 1; c < ncol; c++ {
			sp := specs[c]
			if sp.level == "" { // continuous
				row[c] = ds.Get(r, sp.predictor).Num
			} else if ds.Get(r, sp.predictor).Display() == sp.level {
				row[c] = 1
			}
		}
		dsg.X[k] = row
	}
	return dsg, nil
}

// classLevels returns the sorted distinct display values of a class variable
// over the given rows.
func classLevels(ds *table.Dataset, rows []table.Row, v string) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rows {
		val := ds.Get(r, v)
		if val.IsMissing() {
			continue
		}
		k := val.Display()
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// olsSolve fits y = X*beta by ordinary least squares via the normal equations,
// returning the estimates and their diagnostics. This is the linear-algebra
// seam: a future SAS-compatible variant swaps invert() for a generalized
// (sweep) inverse here.
func olsSolve(X [][]float64, y []float64, p int) (olsFit, error) {
	n := len(y)
	xtx := make([][]float64, p)
	xty := make([]float64, p)
	for i := 0; i < p; i++ {
		xtx[i] = make([]float64, p)
		for k := 0; k < n; k++ {
			xty[i] += X[k][i] * y[k]
			for j := 0; j < p; j++ {
				xtx[i][j] += X[k][i] * X[k][j]
			}
		}
	}
	inv, err := invert(xtx)
	if err != nil {
		return olsFit{}, err
	}
	beta := matVec(inv, xty)

	var sse, sst, ybar float64
	for _, yv := range y {
		ybar += yv
	}
	ybar /= float64(n)
	for k := 0; k < n; k++ {
		pred := 0.0
		for j := 0; j < p; j++ {
			pred += beta[j] * X[k][j]
		}
		sse += (y[k] - pred) * (y[k] - pred)
		sst += (y[k] - ybar) * (y[k] - ybar)
	}
	dfe := n - p
	mse := sse / float64(dfe)

	stderr := make([]float64, p)
	tvalue := make([]float64, p)
	pvalue := make([]float64, p)
	for j := 0; j < p; j++ {
		v := mse * inv[j][j]
		if v > 0 {
			stderr[j] = math.Sqrt(v)
			tvalue[j] = beta[j] / stderr[j]
			pvalue[j] = studentTwoSided(tvalue[j], dfe)
		} else {
			stderr[j] = math.NaN()
			tvalue[j] = math.NaN()
			pvalue[j] = math.NaN()
		}
	}
	r2 := 0.0
	if sst > 0 {
		r2 = 1 - sse/sst
	}
	return olsFit{beta: beta, stderr: stderr, tvalue: tvalue, pvalue: pvalue, rSquare: r2, n: n, dfe: dfe}, nil
}

// studentTwoSided returns the two-sided tail probability Pr(|T| > |t|) for a
// Student-t distribution with df degrees of freedom, using the identity
// Pr(|T| > t) = I_{df/(df+t^2)}(df/2, 1/2) where I is the regularized
// incomplete beta function.
func studentTwoSided(t float64, df int) float64 {
	if df <= 0 {
		return math.NaN()
	}
	x := float64(df) / (float64(df) + t*t)
	return betai(x, float64(df)/2, 0.5)
}

// betai is the regularized incomplete beta function I_x(a, b). It is standard
// numerical mathematics (continued-fraction evaluation via the modified Lentz
// algorithm) implemented from the public definition.
func betai(x, a, b float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	lbeta, _ := math.Lgamma(a + b)
	la, _ := math.Lgamma(a)
	lb, _ := math.Lgamma(b)
	front := math.Exp(lbeta - la - lb + a*math.Log(x) + b*math.Log(1-x))
	if x < (a+1)/(a+b+2) {
		return front * betacf(x, a, b) / a
	}
	return 1 - front*betacf(1-x, b, a)/b
}

// betacf evaluates the continued fraction for the incomplete beta function.
func betacf(x, a, b float64) float64 {
	const (
		maxIter = 200
		eps     = 3e-14
		tiny    = 1e-300
	)
	qab, qap, qam := a+b, a+1, a-1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < tiny {
		d = tiny
	}
	d = 1 / d
	h := d
	for m := 1; m <= maxIter; m++ {
		mf := float64(m)
		// even step
		aa := mf * (b - mf) * x / ((qam + 2*mf) * (a + 2*mf))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		h *= d * c
		// odd step
		aa = -(a + mf) * (qab + mf) * x / ((a + 2*mf) * (qap + 2*mf))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < eps {
			break
		}
	}
	return h
}

// invert returns the inverse of a square matrix via Gauss-Jordan elimination
// with partial pivoting.
func invert(a [][]float64) ([][]float64, error) {
	n := len(a)
	// Augment [a | I].
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, 2*n)
		copy(m[i], a[i])
		m[i][n+i] = 1
	}
	for col := 0; col < n; col++ {
		// Partial pivot.
		piv := col
		for r := col + 1; r < n; r++ {
			if math.Abs(m[r][col]) > math.Abs(m[piv][col]) {
				piv = r
			}
		}
		if math.Abs(m[piv][col]) < 1e-12 {
			return nil, fmt.Errorf("singular matrix (collinear predictors?)")
		}
		m[col], m[piv] = m[piv], m[col]
		// Normalize pivot row.
		d := m[col][col]
		for j := 0; j < 2*n; j++ {
			m[col][j] /= d
		}
		// Eliminate other rows.
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			f := m[r][col]
			for j := 0; j < 2*n; j++ {
				m[r][j] -= f * m[col][j]
			}
		}
	}
	inv := make([][]float64, n)
	for i := range inv {
		inv[i] = append([]float64(nil), m[i][n:]...)
	}
	return inv, nil
}

func matVec(a [][]float64, v []float64) []float64 {
	out := make([]float64, len(a))
	for i := range a {
		for j := range v {
			out[i] += a[i][j] * v[j]
		}
	}
	return out
}

func numOrMissing(f float64) table.Value {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return table.MissingNum()
	}
	return table.Num(f)
}

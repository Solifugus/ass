package proc

import (
	"fmt"
	"math"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() {
	Register("reg", regProc{})
	Register("glm", regProc{}) // GLM with continuous predictors reduces to OLS
}

// regProc implements a basic PROC REG / PROC GLM: ordinary least-squares linear
// regression for `model y = x1 x2 ...;`. It reports the parameter estimates
// (Estimate, StdErr, tValue) and the model R-square. Class effects, multiple
// MODEL statements, and significance probabilities are not implemented.
type regProc struct{}

func (regProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	src, ok := lib.Get(step.Data)
	if !ok {
		logger.Error("PROC REG: data set %s not found.", strings.ToUpper(step.Data))
		return nil
	}
	var model *ast.ModelStatement
	for _, s := range step.Body {
		if m, ok := s.(*ast.ModelStatement); ok {
			model = m
			break
		}
	}
	if model == nil {
		logger.Error("PROC REG: a MODEL statement is required.")
		return nil
	}

	fit, err := ols(src, model.Response, model.Predictors)
	if err != nil {
		logger.Error("PROC REG: %v", err)
		return nil
	}

	result := table.NewDataset("", "_reg_")
	result.AddColumn(table.Column{Name: "Variable", Kind: table.Character})
	result.AddColumn(table.Column{Name: "DF", Kind: table.Numeric})
	result.AddColumn(table.Column{Name: "Estimate", Kind: table.Numeric, Format: "12.5"})
	result.AddColumn(table.Column{Name: "StdErr", Kind: table.Numeric, Format: "12.5"})
	result.AddColumn(table.Column{Name: "tValue", Kind: table.Numeric, Format: "8.2"})
	result.AddColumn(table.Column{Name: "Pr>|t|", Kind: table.Numeric, Format: "7.4"})

	names := append([]string{"Intercept"}, model.Predictors...)
	for i, nm := range names {
		result.AppendRow(table.Row{
			"variable": table.Char(nm),
			"df":       table.Num(1),
			"estimate": table.Num(fit.beta[i]),
			"stderr":   numOrMissing(fit.stderr[i]),
			"tvalue":   numOrMissing(fit.tvalue[i]),
			"pr>|t|":   numOrMissing(fit.pvalue[i]),
		})
	}

	fmt.Printf("Dependent Variable: %s\n", model.Response)
	fmt.Printf("R-Square: %.5f   Observations: %d\n\n", fit.rSquare, fit.n)
	fmt.Print(renderListing(result, printOptions{}))
	return nil
}

// olsFit holds a fitted model's coefficients and diagnostics.
type olsFit struct {
	beta    []float64
	stderr  []float64
	tvalue  []float64
	pvalue  []float64 // two-sided Pr > |t|
	rSquare float64
	n       int
	dfe     int
}

// ols fits y = b0 + b1*x1 + ... by ordinary least squares via the normal
// equations. Only complete cases (no missing in response/predictors) are used.
func ols(ds *table.Dataset, response string, predictors []string) (olsFit, error) {
	p := len(predictors) + 1 // + intercept
	var X [][]float64
	var y []float64
	for _, r := range ds.Rows {
		yv := ds.Get(r, response)
		if yv.IsMissing() {
			continue
		}
		row := make([]float64, p)
		row[0] = 1
		ok := true
		for j, xv := range predictors {
			v := ds.Get(r, xv)
			if v.IsMissing() {
				ok = false
				break
			}
			row[j+1] = v.Num
		}
		if !ok {
			continue
		}
		X = append(X, row)
		y = append(y, yv.Num)
	}
	n := len(y)
	if n <= p {
		return olsFit{}, fmt.Errorf("not enough complete observations (%d) for %d parameters", n, p)
	}

	// Normal equations: (X'X) beta = X'y.
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

	// Residuals, SSE, SST.
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
	front := math.Exp(lbeta-la-lb+a*math.Log(x)+b*math.Log(1-x))
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

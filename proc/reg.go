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

	names := append([]string{"Intercept"}, model.Predictors...)
	for i, nm := range names {
		result.AppendRow(table.Row{
			"variable": table.Char(nm),
			"df":       table.Num(1),
			"estimate": table.Num(fit.beta[i]),
			"stderr":   numOrMissing(fit.stderr[i]),
			"tvalue":   numOrMissing(fit.tvalue[i]),
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
	rSquare float64
	n       int
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
	for j := 0; j < p; j++ {
		v := mse * inv[j][j]
		if v > 0 {
			stderr[j] = math.Sqrt(v)
			tvalue[j] = beta[j] / stderr[j]
		} else {
			stderr[j] = math.NaN()
			tvalue[j] = math.NaN()
		}
	}
	r2 := 0.0
	if sst > 0 {
		r2 = 1 - sse/sst
	}
	return olsFit{beta: beta, stderr: stderr, tvalue: tvalue, rSquare: r2, n: n}, nil
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

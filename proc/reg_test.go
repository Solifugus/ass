package proc

import (
	"math"
	"testing"

	"github.com/solifugus/ass/table"
)

func xyDS(xs, ys []float64) *table.Dataset {
	ds := table.NewDataset("", "xy")
	ds.AddColumn(table.Column{Name: "x", Kind: table.Numeric})
	ds.AddColumn(table.Column{Name: "y", Kind: table.Numeric})
	for i := range xs {
		ds.AppendRow(table.Row{"x": table.Num(xs[i]), "y": table.Num(ys[i])})
	}
	return ds
}

func TestOLSPerfectLine(t *testing.T) {
	// y = 2x + 1 exactly.
	fit, err := ols(xyDS([]float64{1, 2, 3, 4, 5}, []float64{3, 5, 7, 9, 11}), "y", []string{"x"})
	if err != nil {
		t.Fatalf("ols: %v", err)
	}
	if math.Abs(fit.beta[0]-1) > 1e-9 || math.Abs(fit.beta[1]-2) > 1e-9 {
		t.Errorf("beta = %v, want [1 2]", fit.beta)
	}
	if math.Abs(fit.rSquare-1) > 1e-9 {
		t.Errorf("R^2 = %v, want 1", fit.rSquare)
	}
}

func TestOLSKnownFit(t *testing.T) {
	// Hand-computed OLS: slope 0.8, intercept 0.6.
	fit, err := ols(xyDS([]float64{1, 2, 3, 4, 5}, []float64{1, 3, 2, 5, 4}), "y", []string{"x"})
	if err != nil {
		t.Fatalf("ols: %v", err)
	}
	if math.Abs(fit.beta[0]-0.6) > 1e-9 {
		t.Errorf("intercept = %v, want 0.6", fit.beta[0])
	}
	if math.Abs(fit.beta[1]-0.8) > 1e-9 {
		t.Errorf("slope = %v, want 0.8", fit.beta[1])
	}
	if math.IsNaN(fit.stderr[1]) || fit.stderr[1] <= 0 {
		t.Errorf("slope stderr = %v, want a positive number", fit.stderr[1])
	}
}

func TestOLSMultipleRegression(t *testing.T) {
	// y = 1 + 2*x1 + 3*x2 exactly.
	ds := table.NewDataset("", "d")
	ds.AddColumn(table.Column{Name: "y", Kind: table.Numeric})
	ds.AddColumn(table.Column{Name: "x1", Kind: table.Numeric})
	ds.AddColumn(table.Column{Name: "x2", Kind: table.Numeric})
	rows := [][3]float64{{0, 0}, {1, 0}, {0, 1}, {1, 1}, {2, 1}, {3, 2}}
	for _, r := range rows {
		x1, x2 := r[0], r[1]
		y := 1 + 2*x1 + 3*x2
		ds.AppendRow(table.Row{"y": table.Num(y), "x1": table.Num(x1), "x2": table.Num(x2)})
	}
	fit, err := ols(ds, "y", []string{"x1", "x2"})
	if err != nil {
		t.Fatalf("ols: %v", err)
	}
	want := []float64{1, 2, 3}
	for i, w := range want {
		if math.Abs(fit.beta[i]-w) > 1e-9 {
			t.Errorf("beta[%d] = %v, want %v", i, fit.beta[i], w)
		}
	}
}

func TestOLSTooFewObs(t *testing.T) {
	if _, err := ols(xyDS([]float64{1}, []float64{2}), "y", []string{"x"}); err == nil {
		t.Error("expected error for too few observations")
	}
}

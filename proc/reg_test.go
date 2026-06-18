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

func TestStudentTwoSided(t *testing.T) {
	// Known two-tailed Student-t tail probabilities (cross-checked against R's
	// 2*pt(-|t|, df)).
	cases := []struct {
		t    float64
		df   int
		want float64
	}{
		{2.3094, 3, 0.1041},
		{0.5222, 3, 0.6376},
		{0, 5, 1.0},
		{2.5706, 5, 0.0500}, // the 5% two-sided critical value for 5 df
	}
	for _, c := range cases {
		got := studentTwoSided(c.t, c.df)
		if math.Abs(got-c.want) > 5e-4 {
			t.Errorf("studentTwoSided(%v, %d) = %.4f, want %.4f", c.t, c.df, got, c.want)
		}
	}
}

func TestRegPValuesFromFit(t *testing.T) {
	fit, err := ols(xyDS([]float64{1, 2, 3, 4, 5}, []float64{1, 3, 2, 5, 4}), "y", []string{"x"})
	if err != nil {
		t.Fatalf("ols: %v", err)
	}
	if fit.dfe != 3 {
		t.Errorf("dfe = %d, want 3", fit.dfe)
	}
	// slope t≈2.31, df=3 -> p≈0.1041
	if math.Abs(fit.pvalue[1]-0.1041) > 1e-3 {
		t.Errorf("slope p = %.4f, want ~0.1041", fit.pvalue[1])
	}
}

func classDS() *table.Dataset {
	// y = (10 if A, 15 if B, 20 if C) + 2*x, exactly.
	ds := table.NewDataset("", "trial")
	ds.AddColumn(table.Column{Name: "grp", Kind: table.Character})
	ds.AddColumn(table.Column{Name: "x", Kind: table.Numeric})
	ds.AddColumn(table.Column{Name: "y", Kind: table.Numeric})
	rows := []struct {
		g string
		x float64
	}{{"A", 1}, {"A", 2}, {"B", 1}, {"B", 3}, {"C", 2}, {"C", 5}}
	base := map[string]float64{"A": 10, "B": 15, "C": 20}
	for _, r := range rows {
		y := base[r.g] + 2*r.x
		ds.AppendRow(table.Row{"grp": table.Char(r.g), "x": table.Num(r.x), "y": table.Num(y)})
	}
	return ds
}

func TestGLMClassReferenceCoding(t *testing.T) {
	fit, dsg, err := fitModel(classDS(), "y", []string{"grp", "x"}, []string{"grp"})
	if err != nil {
		t.Fatalf("fitModel: %v", err)
	}
	// Parameter rows: Intercept, grp A, grp B, grp C (ref), x.
	wantLabels := []string{"Intercept", "grp A", "grp B", "grp C", "x"}
	if len(dsg.params) != len(wantLabels) {
		t.Fatalf("got %d params, want %d: %+v", len(dsg.params), len(wantLabels), dsg.params)
	}
	for i, w := range wantLabels {
		if dsg.params[i].label != w {
			t.Errorf("param %d label = %q, want %q", i, dsg.params[i].label, w)
		}
	}
	// The last grp level (C) must be the reference (estimate fixed at 0).
	if !dsg.params[3].reference {
		t.Errorf("grp C should be the reference level: %+v", dsg.params[3])
	}
	for i, p := range dsg.params {
		if p.reference != (i == 3) {
			t.Errorf("param %d reference = %v, unexpected", i, p.reference)
		}
	}
	// Reference-cell estimates: Intercept = C baseline (20), grp A = A-C = -10,
	// grp B = B-C = -5, x slope = 2. (C reference is 0, not a solved column.)
	want := map[string]float64{"Intercept": 20, "grp A": -10, "grp B": -5, "x": 2}
	for _, p := range dsg.params {
		if p.reference {
			continue
		}
		if math.Abs(fit.beta[p.col]-want[p.label]) > 1e-6 {
			t.Errorf("%s estimate = %v, want %v", p.label, fit.beta[p.col], want[p.label])
		}
	}
	if math.Abs(fit.rSquare-1) > 1e-9 {
		t.Errorf("R^2 = %v, want 1 (exact linear fit)", fit.rSquare)
	}
}

func TestGLMClassExcludesMissingLevels(t *testing.T) {
	ds := classDS()
	// Add a row with a missing class value; it must be dropped from the fit.
	ds.AppendRow(table.Row{"grp": table.MissingChar(), "x": table.Num(9), "y": table.Num(99)})
	fit, _, err := fitModel(ds, "y", []string{"grp", "x"}, []string{"grp"})
	if err != nil {
		t.Fatalf("fitModel: %v", err)
	}
	if fit.n != 6 {
		t.Errorf("n = %d, want 6 (missing-class row excluded)", fit.n)
	}
}

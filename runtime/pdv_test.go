package runtime

import (
	"testing"

	"github.com/solifugus/ass/table"
)

func TestPDVSetGet(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("Age", table.Num(25))
	// Case-insensitive lookup.
	if got := pdv.Get("AGE"); got.Num != 25 {
		t.Errorf("Get AGE = %v, want 25", got.Display())
	}
	if !pdv.Has("age") {
		t.Error("Has should be case-insensitive")
	}
}

func TestPDVUndeclaredIsNumericMissing(t *testing.T) {
	pdv := NewPDV()
	got := pdv.Get("nope")
	if !got.IsMissing() || got.Kind != table.Numeric {
		t.Errorf("undeclared var = %v, want numeric missing", got.Display())
	}
}

func TestPDVDeclaredTypedMissing(t *testing.T) {
	pdv := NewPDV()
	pdv.Declare("name", table.Character)
	got := pdv.Get("name")
	if !got.IsMissing() || got.Kind != table.Character {
		t.Errorf("declared char var = %v, want char missing", got.Display())
	}
}

func TestPDVTypeFixedAtFirstUse(t *testing.T) {
	pdv := NewPDV()
	pdv.Declare("x", table.Numeric)
	if k, _ := pdv.Kind("x"); k != table.Numeric {
		t.Fatalf("kind = %v, want numeric", k)
	}
	// Re-declaring with a different type does not change it.
	pdv.Declare("x", table.Character)
	if k, _ := pdv.Kind("x"); k != table.Numeric {
		t.Errorf("kind after re-declare = %v, want numeric (fixed at first use)", k)
	}
}

func TestPDVOrderPreserved(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("name", table.Char("x"))
	pdv.Set("age", table.Num(1))
	pdv.Set("city", table.Char("y"))
	names := pdv.Names()
	want := []string{"name", "age", "city"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestPDVResetVars(t *testing.T) {
	pdv := NewPDV()
	pdv.Set("a", table.Num(1))
	pdv.Set("b", table.Char("hi"))
	pdv.ResetVars()
	if got := pdv.Get("a"); !got.IsMissing() {
		t.Errorf("a after reset = %v, want missing", got.Display())
	}
	// Declaration/type and order survive the reset.
	if got := pdv.Get("b"); !got.IsMissing() || got.Kind != table.Character {
		t.Errorf("b after reset = %v, want char missing", got.Display())
	}
	if len(pdv.Names()) != 2 {
		t.Errorf("names after reset = %v, want 2 declared", pdv.Names())
	}
}

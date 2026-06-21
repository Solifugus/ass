package table

import "testing"

func TestUserInformatParse(t *testing.T) {
	// Numeric-result informat with string keys + other.
	grade := &UserInformat{Name: "grade", Ranges: []InformatRange{
		{Key: "A", Result: Num(4)},
		{Key: "B", Result: Num(3)},
		{Other: true, Result: Num(0)},
	}}
	if v, ok := grade.Parse("A"); !ok || v.Num != 4 {
		t.Errorf("grade A = %v,%v want 4", v.Display(), ok)
	}
	if v, ok := grade.Parse("Z"); !ok || v.Num != 0 {
		t.Errorf("grade Z (other) = %v,%v want 0", v.Display(), ok)
	}

	// Character-result informat.
	resp := &UserInformat{Name: "$resp", Char: true, Ranges: []InformatRange{
		{Key: "Y", Result: Char("Yes")},
		{Key: "N", Result: Char("No")},
	}}
	if v, ok := resp.Parse("Y"); !ok || v.Str != "Yes" {
		t.Errorf("resp Y = %v,%v want Yes", v.Display(), ok)
	}

	// Numeric range informat.
	band := &UserInformat{Name: "band", Ranges: []InformatRange{
		{Numeric: true, Low: 1, High: 10, Result: Num(1)},
		{Numeric: true, Low: 11, High: 20, Result: Num(2)},
		{Other: true, Result: Num(9)},
	}}
	if v, _ := band.Parse("5"); v.Num != 1 {
		t.Errorf("band 5 = %v want 1", v.Display())
	}
	if v, _ := band.Parse("15"); v.Num != 2 {
		t.Errorf("band 15 = %v want 2", v.Display())
	}
	if v, _ := band.Parse("99"); v.Num != 9 {
		t.Errorf("band 99 (other) = %v want 9", v.Display())
	}
}

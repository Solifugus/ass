package table

import "testing"

func TestValueMissing(t *testing.T) {
	if !MissingNum().IsMissing() {
		t.Error("MissingNum should be missing")
	}
	if !MissingChar().IsMissing() {
		t.Error("MissingChar (empty string) should be missing")
	}
	if Num(0).IsMissing() {
		t.Error("numeric 0 is not missing")
	}
	if Char("x").IsMissing() {
		t.Error("non-empty char is not missing")
	}
	if Char(" ").IsMissing() {
		// A single space is technically not the empty string; SAS treats blank
		// as missing, but we only treat "" as missing for now. Document intent.
		t.Skip("blank-as-missing not yet normalized; see formats phase")
	}
}

func TestValueDisplay(t *testing.T) {
	cases := []struct {
		v    Value
		want string
	}{
		{Num(25), "25"},
		{Num(3.5), "3.5"},
		{MissingNum(), "."},
		{Char("John"), "John"},
		{MissingChar(), ""},
	}
	for _, c := range cases {
		if got := c.v.Display(); got != c.want {
			t.Errorf("Display() = %q, want %q", got, c.want)
		}
	}
}

func TestDatasetColumnsAndGet(t *testing.T) {
	d := NewDataset("", "people")
	if d.Lib != "WORK" {
		t.Errorf("default lib = %q, want WORK", d.Lib)
	}
	d.AddColumn(Column{Name: "name", Kind: Character})
	d.AddColumn(Column{Name: "age", Kind: Numeric})
	if d.AddColumn(Column{Name: "Name", Kind: Numeric}) {
		t.Error("duplicate column (case-insensitive) should not be added")
	}
	if len(d.Columns) != 2 {
		t.Fatalf("columns = %d, want 2", len(d.Columns))
	}

	d.AppendRow(Row{"name": Char("John"), "age": Num(25)})
	r := d.Rows[0]
	if got := d.Get(r, "AGE"); got.Num != 25 {
		t.Errorf("Get AGE = %v, want 25", got)
	}
	// Absent column returns typed missing.
	if got := d.Get(r, "height"); !got.IsMissing() || got.Kind != Numeric {
		t.Errorf("Get unknown column = %v, want numeric missing", got)
	}
	if got := d.Get(r, "name"); got.Str != "John" {
		t.Errorf("Get name = %v, want John", got)
	}
}

func TestLibraryPutGet(t *testing.T) {
	lib := NewLibrary()
	d := NewDataset("", "people")
	lib.Put(d)
	if !lib.Has("PEOPLE") {
		t.Error("Has should be case-insensitive")
	}
	if _, ok := lib.Get("work.people"); !ok {
		t.Error("Get should accept qualified lib.name")
	}
	// Overwrite.
	d2 := NewDataset("", "people")
	d2.AddColumn(Column{Name: "x", Kind: Numeric})
	lib.Put(d2)
	got, _ := lib.Get("people")
	if len(got.Columns) != 1 {
		t.Errorf("Put should overwrite; columns = %d, want 1", len(got.Columns))
	}
}

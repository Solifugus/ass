package parser

import (
	"testing"

	"github.com/solifugus/ass/ast"
)

// dataBody parses a single DATA step and returns its body statements.
func dataBody(t *testing.T, src string) []ast.Statement {
	t.Helper()
	p := New(src)
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parse errors for %q: %v", src, errs)
	}
	ds, ok := prog.Steps[0].(*ast.DataStep)
	if !ok {
		t.Fatalf("step 0 is %T, want *ast.DataStep", prog.Steps[0])
	}
	return ds.Body
}

func TestParseRenameStatement(t *testing.T) {
	body := dataBody(t, "data b; set a; rename x=alpha Y=Beta; run;")
	var rs *ast.RenameStatement
	for _, s := range body {
		if r, ok := s.(*ast.RenameStatement); ok {
			rs = r
		}
	}
	if rs == nil {
		t.Fatalf("no RenameStatement parsed; body = %v", body)
	}
	if rs.Map["x"] != "alpha" || rs.Map["y"] != "Beta" {
		t.Errorf("rename map = %v, want x->alpha y->Beta (keys lowercased)", rs.Map)
	}
}

// TestParseRenameAsVariable confirms `rename = expr;` is still an assignment to a
// variable named rename (rename is not a reserved word in SAS).
func TestParseRenameAsVariable(t *testing.T) {
	body := dataBody(t, "data b; rename = 5; run;")
	if _, ok := body[0].(*ast.AssignmentStatement); !ok {
		t.Fatalf("stmt 0 is %T, want AssignmentStatement (rename used as a var name)", body[0])
	}
}

func TestParseTablesOut(t *testing.T) {
	// `/ out=<name>` is captured into Out (not treated as an option), while other
	// options after the slash are still recorded.
	p := New("proc freq data=s; tables a*b / out=counts chisq; run;")
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	ps, ok := prog.Steps[0].(*ast.ProcStep)
	if !ok {
		t.Fatalf("step 0 is %T, want *ast.ProcStep", prog.Steps[0])
	}
	var ts *ast.TablesStatement
	for _, s := range ps.Body {
		if t2, ok := s.(*ast.TablesStatement); ok {
			ts = t2
		}
	}
	if ts == nil {
		t.Fatalf("no TablesStatement parsed; body = %v", ps.Body)
	}
	if ts.Out != "counts" {
		t.Errorf("Out = %q, want counts", ts.Out)
	}
	if !ts.HasOption("chisq") {
		t.Errorf("chisq option lost; options = %v", ts.Options)
	}
	if ts.HasOption("out") {
		t.Errorf("out should be captured into Out, not left as an option: %v", ts.Options)
	}
}

func TestParseMeansOutput(t *testing.T) {
	p := New("proc means data=s; var x y; output out=stats mean=mx my sum=sx sy; run;")
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	ps := prog.Steps[0].(*ast.ProcStep)
	var os *ast.MeansOutputStatement
	for _, s := range ps.Body {
		if o, ok := s.(*ast.MeansOutputStatement); ok {
			os = o
		}
	}
	if os == nil {
		t.Fatalf("no MeansOutputStatement parsed; body = %v", ps.Body)
	}
	if os.Out != "stats" {
		t.Errorf("Out = %q, want stats", os.Out)
	}
	if len(os.Stats) != 2 {
		t.Fatalf("got %d stat clauses, want 2: %+v", len(os.Stats), os.Stats)
	}
	if os.Stats[0].Stat != "mean" || len(os.Stats[0].Names) != 2 || os.Stats[0].Names[0] != "mx" {
		t.Errorf("clause 0 = %+v, want mean=[mx my]", os.Stats[0])
	}
	if os.Stats[1].Stat != "sum" || os.Stats[1].Names[1] != "sy" {
		t.Errorf("clause 1 = %+v, want sum=[sx sy]", os.Stats[1])
	}
}

func TestParseAssignmentAndInput(t *testing.T) {
	body := dataBody(t, "data s; input item $ qty price; total = qty * price; run;")
	if len(body) != 2 {
		t.Fatalf("got %d statements, want 2: %v", len(body), body)
	}
	in, ok := body[0].(*ast.InputStatement)
	if !ok {
		t.Fatalf("stmt 0 is %T, want InputStatement", body[0])
	}
	if len(in.Vars) != 3 || !in.Vars[0].Char || in.Vars[1].Char {
		t.Errorf("input vars wrong: %+v", in.Vars)
	}
	asg, ok := body[1].(*ast.AssignmentStatement)
	if !ok {
		t.Fatalf("stmt 1 is %T, want AssignmentStatement", body[1])
	}
	if asg.Name != "total" || asg.Value.String() != "(qty * price)" {
		t.Errorf("assignment = %s = %s", asg.Name, asg.Value.String())
	}
}

func TestParseInputTrailingAt(t *testing.T) {
	cases := []struct {
		src      string
		wantHold int
		wantVars int
	}{
		{"data s; input x @@; run;", 2, 1},
		{"data s; input x@@; run;", 2, 1},
		{"data s; input type $ @; run;", 1, 1},
		{"data s; input a b; run;", 0, 2},
		{"data s; input @3 x 2.; run;", 0, 1}, // leading @n pointer is not a hold
	}
	for _, c := range cases {
		in := dataBody(t, c.src)[0].(*ast.InputStatement)
		if in.TrailingAt != c.wantHold {
			t.Errorf("%q: TrailingAt = %d, want %d", c.src, in.TrailingAt, c.wantHold)
		}
		if len(in.Vars) != c.wantVars {
			t.Errorf("%q: vars = %d, want %d (%+v)", c.src, len(in.Vars), c.wantVars, in.Vars)
		}
	}
}

func TestParseInputLinePointer(t *testing.T) {
	in := dataBody(t, "data s; input #1 name $ age #2 city $ zip; run;")[0].(*ast.InputStatement)
	if len(in.Vars) != 4 {
		t.Fatalf("vars = %d, want 4 (%+v)", len(in.Vars), in.Vars)
	}
	// `#1` precedes name; the next #n (`#2`) precedes city. age and zip inherit.
	want := []int{1, 0, 2, 0}
	for i, w := range want {
		if in.Vars[i].Line != w {
			t.Errorf("var %d (%s): Line = %d, want %d", i, in.Vars[i].Name, in.Vars[i].Line, w)
		}
	}
	if !in.Vars[0].Char || in.Vars[1].Char || !in.Vars[2].Char || in.Vars[3].Char {
		t.Errorf("char flags wrong: %+v", in.Vars)
	}
}

func TestParsePutLinePointer(t *testing.T) {
	body := dataBody(t, `data _null_; put #1 a b #2 c; run;`)
	put := body[0].(*ast.PutStatement)
	if len(put.Items) != 3 {
		t.Fatalf("items = %d, want 3 (%+v)", len(put.Items), put.Items)
	}
	want := []int{1, 0, 2}
	for i, w := range want {
		if put.Items[i].Line != w {
			t.Errorf("item %d: Line = %d, want %d", i, put.Items[i].Line, w)
		}
	}
}

func TestParsePutTrailingAt(t *testing.T) {
	cases := []struct {
		src  string
		want int
		n    int // expected item count
	}{
		{`data _null_; put x @@; run;`, 2, 1},
		{`data _null_; put x @; run;`, 1, 1},
		{`data _null_; put x@@; run;`, 2, 1},
		{`data _null_; put x@; run;`, 1, 1},
		{`data _null_; put name age; run;`, 0, 2},
		{`data _null_; put @5 x; run;`, 0, 1}, // leading @n is not a trailing hold
	}
	for _, c := range cases {
		body := dataBody(t, c.src)
		put := body[0].(*ast.PutStatement)
		if put.TrailingAt != c.want {
			t.Errorf("%q: TrailingAt = %d, want %d", c.src, put.TrailingAt, c.want)
		}
		if len(put.Items) != c.n {
			t.Errorf("%q: items = %d, want %d (%+v)", c.src, len(put.Items), c.n, put.Items)
		}
	}
}

func TestParseColumnAndPointerInput(t *testing.T) {
	body := dataBody(t, "data s; input name $ 1-10 age 11-13 @1 id $5. +2 x 3.; run;")
	in, ok := body[0].(*ast.InputStatement)
	if !ok {
		t.Fatalf("stmt 0 is %T, want InputStatement", body[0])
	}
	if len(in.Vars) != 4 {
		t.Fatalf("got %d vars, want 4: %+v", len(in.Vars), in.Vars)
	}
	if v := in.Vars[0]; !v.Char || v.ColStart != 1 || v.ColEnd != 10 {
		t.Errorf("var0 = %+v, want name $ 1-10", v)
	}
	if v := in.Vars[1]; v.Char || v.ColStart != 11 || v.ColEnd != 13 {
		t.Errorf("var1 = %+v, want age 11-13", v)
	}
	if v := in.Vars[2]; v.At != 1 || v.Informat != "$5." || !v.Char {
		t.Errorf("var2 = %+v, want @1 id $5.", v)
	}
	if v := in.Vars[3]; v.Plus != 2 || v.Informat != "3." {
		t.Errorf("var3 = %+v, want +2 x 3.", v)
	}
}

func TestParseSetAndSubsettingIf(t *testing.T) {
	body := dataBody(t, "data adults; set people; if age >= 18; run;")
	if _, ok := body[0].(*ast.SetStatement); !ok {
		t.Fatalf("stmt 0 is %T, want SetStatement", body[0])
	}
	si, ok := body[1].(*ast.SubsettingIf)
	if !ok {
		t.Fatalf("stmt 1 is %T, want SubsettingIf", body[1])
	}
	if si.Condition.String() != "(age >= 18)" {
		t.Errorf("subsetting cond = %s", si.Condition.String())
	}
}

func TestParseIfThenElse(t *testing.T) {
	body := dataBody(t, "data g; if s >= 90 then grade = 'A'; else grade = 'B'; run;")
	ifs, ok := body[0].(*ast.IfStatement)
	if !ok {
		t.Fatalf("stmt 0 is %T, want IfStatement", body[0])
	}
	if _, ok := ifs.Consequence.(*ast.AssignmentStatement); !ok {
		t.Errorf("consequence is %T, want AssignmentStatement", ifs.Consequence)
	}
	if ifs.Alternative == nil {
		t.Errorf("expected else branch")
	}
}

func TestParseDoLoopWithOutput(t *testing.T) {
	body := dataBody(t, "data sq; do i = 1 to 5; x = i * i; output; end; run;")
	do, ok := body[0].(*ast.DoStatement)
	if !ok {
		t.Fatalf("stmt 0 is %T, want DoStatement", body[0])
	}
	if do.Kind != ast.DoIterative || do.Var != "i" {
		t.Errorf("do kind/var = %d/%s", do.Kind, do.Var)
	}
	if do.From.String() != "1" || do.To.String() != "5" {
		t.Errorf("do range = %s..%s", do.From.String(), do.To.String())
	}
	if len(do.Body) != 2 {
		t.Fatalf("do body = %d statements, want 2", len(do.Body))
	}
	if _, ok := do.Body[1].(*ast.OutputStatement); !ok {
		t.Errorf("do body[1] is %T, want OutputStatement", do.Body[1])
	}
}

func TestParseProcByAndVar(t *testing.T) {
	p := New("proc sort data=people; by descending age; run; proc print data=people; var name age; run;")
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	sort := prog.Steps[0].(*ast.ProcStep)
	by, ok := sort.Body[0].(*ast.ByStatement)
	if !ok {
		t.Fatalf("sort body[0] is %T, want ByStatement", sort.Body[0])
	}
	if len(by.Vars) != 1 || by.Vars[0] != "age" || !by.Descending[0] {
		t.Errorf("by = %+v desc=%+v", by.Vars, by.Descending)
	}
	print := prog.Steps[1].(*ast.ProcStep)
	v, ok := print.Body[0].(*ast.VarStatement)
	if !ok {
		t.Fatalf("print body[0] is %T, want VarStatement", print.Body[0])
	}
	if len(v.Vars) != 2 {
		t.Errorf("var list = %v", v.Vars)
	}
}

func TestParseValueStatement(t *testing.T) {
	src := `proc format;
  value agegrp low - 12 = 'Child' 13 - 19 = 'Teen' 20 - high = 'Adult';
  value $sex 'M' = 'Male' 'F' = 'Female' other = 'Unknown';
  value g 0 <- 10 = 'A' 10 -< 20 = 'B';
  value mult 1,3,5 = 'Odd';
run;`
	p := New(src)
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	ps, ok := prog.Steps[0].(*ast.ProcStep)
	if !ok || ps.Name != "format" {
		t.Fatalf("step 0 is %T (name %q), want proc format", prog.Steps[0], ps.Name)
	}
	var vals []*ast.ValueStatement
	for _, s := range ps.Body {
		if v, ok := s.(*ast.ValueStatement); ok {
			vals = append(vals, v)
		}
	}
	if len(vals) != 4 {
		t.Fatalf("got %d value statements, want 4", len(vals))
	}

	age := vals[0]
	if age.Name != "agegrp" || age.Char {
		t.Errorf("agegrp: name=%q char=%v", age.Name, age.Char)
	}
	if len(age.Ranges) != 3 || !age.Ranges[0].NoLow || age.Ranges[0].High != "12" || age.Ranges[0].Label != "Child" {
		t.Errorf("agegrp ranges wrong: %+v", age.Ranges)
	}
	if !age.Ranges[2].NoHigh || age.Ranges[2].Low != "20" {
		t.Errorf("agegrp adult range wrong: %+v", age.Ranges[2])
	}

	sex := vals[1]
	if !sex.Char || sex.Name != "$sex" {
		t.Errorf("$sex: name=%q char=%v", sex.Name, sex.Char)
	}
	if len(sex.Ranges) != 3 || !sex.Ranges[2].Other || sex.Ranges[2].Label != "Unknown" {
		t.Errorf("$sex ranges wrong: %+v", sex.Ranges)
	}

	g := vals[2]
	if len(g.Ranges) != 2 || !g.Ranges[0].LowExcl || !g.Ranges[1].HighExcl {
		t.Errorf("g exclusive bounds wrong: %+v", g.Ranges)
	}

	mult := vals[3]
	if len(mult.Ranges) != 3 {
		t.Fatalf("mult comma list: got %d ranges, want 3: %+v", len(mult.Ranges), mult.Ranges)
	}
	for _, r := range mult.Ranges {
		if r.Label != "Odd" || r.Low != r.High {
			t.Errorf("mult range wrong: %+v", r)
		}
	}
}

func TestParsePictureStatement(t *testing.T) {
	src := `proc format;
  picture dollars low-high = '000,000,009.99' (prefix='$');
  picture zfill   other    = '000000' (mult=1 fill='0');
run;`
	p := New(src)
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	ps := prog.Steps[0].(*ast.ProcStep)
	var vals []*ast.ValueStatement
	for _, s := range ps.Body {
		if v, ok := s.(*ast.ValueStatement); ok {
			vals = append(vals, v)
		}
	}
	if len(vals) != 2 {
		t.Fatalf("got %d picture statements, want 2", len(vals))
	}
	d := vals[0]
	if !d.Picture || d.Name != "dollars" {
		t.Errorf("dollars: picture=%v name=%q", d.Picture, d.Name)
	}
	if len(d.Ranges) != 1 || d.Ranges[0].Label != "000,000,009.99" || d.Ranges[0].Prefix != "$" {
		t.Errorf("dollars range wrong: %+v", d.Ranges)
	}
	z := vals[1]
	if !z.Picture || z.Ranges[0].Mult != "1" || z.Ranges[0].Fill != "0" {
		t.Errorf("zfill options wrong: %+v", z.Ranges)
	}
}

func TestParseTablesCrossing(t *testing.T) {
	src := `proc freq data=d; tables a b c*d / nocol; run;`
	p := New(src)
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	ps := prog.Steps[0].(*ast.ProcStep)
	var tab *ast.TablesStatement
	for _, s := range ps.Body {
		if ts, ok := s.(*ast.TablesStatement); ok {
			tab = ts
		}
	}
	if tab == nil {
		t.Fatal("no TablesStatement parsed")
	}
	if len(tab.Requests) != 3 {
		t.Fatalf("requests = %v, want 3 (a, b, c*d)", tab.Requests)
	}
	if len(tab.Requests[0]) != 1 || tab.Requests[0][0] != "a" {
		t.Errorf("request 0 = %v, want [a]", tab.Requests[0])
	}
	if len(tab.Requests[2]) != 2 || tab.Requests[2][0] != "c" || tab.Requests[2][1] != "d" {
		t.Errorf("request 2 = %v, want [c d]", tab.Requests[2])
	}
}

func TestParseInputInformats(t *testing.T) {
	body := dataBody(t, "data t; input id name $ pay : comma8. d date9.; run;")
	in, ok := body[0].(*ast.InputStatement)
	if !ok {
		t.Fatalf("stmt 0 is %T, want InputStatement", body[0])
	}
	if len(in.Vars) != 4 {
		t.Fatalf("got %d vars, want 4: %+v", len(in.Vars), in.Vars)
	}
	if in.Vars[0].Name != "id" || in.Vars[0].Informat != "" {
		t.Errorf("id: %+v", in.Vars[0])
	}
	if in.Vars[1].Name != "name" || !in.Vars[1].Char {
		t.Errorf("name: %+v", in.Vars[1])
	}
	if in.Vars[2].Name != "pay" || in.Vars[2].Informat != "comma8." {
		t.Errorf("pay: %+v", in.Vars[2])
	}
	if in.Vars[3].Name != "d" || in.Vars[3].Informat != "date9." {
		t.Errorf("d: %+v", in.Vars[3])
	}
}

func TestParseDatasetOptions(t *testing.T) {
	body := dataBody(t, "data out; set a(where=(x>0) keep=x y rename=(x=z) in=ina); run;")
	set, ok := body[0].(*ast.SetStatement)
	if !ok {
		t.Fatalf("stmt 0 is %T, want SetStatement", body[0])
	}
	if len(set.Refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(set.Refs))
	}
	ref := set.Refs[0]
	if ref.Name != "a" {
		t.Errorf("name = %q, want a", ref.Name)
	}
	if ref.In != "ina" {
		t.Errorf("in = %q, want ina", ref.In)
	}
	o := ref.Options
	if o == nil {
		t.Fatal("options nil")
	}
	if len(o.Keep) != 2 || o.Keep[0] != "x" || o.Keep[1] != "y" {
		t.Errorf("keep = %v, want [x y]", o.Keep)
	}
	if o.Rename["x"] != "z" {
		t.Errorf("rename = %v, want x->z", o.Rename)
	}
	if o.Where == nil {
		t.Error("where not parsed")
	}
}

func TestParseInfileOptions(t *testing.T) {
	body := dataBody(t, `data t; infile "data.csv" dsd dlm=";" firstobs=2 obs=10 missover; input x y; run;`)
	in, ok := body[0].(*ast.InfileStatement)
	if !ok {
		t.Fatalf("stmt 0 is %T, want *ast.InfileStatement", body[0])
	}
	if in.Path != "data.csv" {
		t.Errorf("Path = %q, want data.csv", in.Path)
	}
	if !in.DSD {
		t.Error("DSD = false, want true")
	}
	if in.Delimiter != ";" {
		t.Errorf("Delimiter = %q, want ;", in.Delimiter)
	}
	if in.Firstobs != 2 || in.Obs != 10 {
		t.Errorf("Firstobs/Obs = %d/%d, want 2/10", in.Firstobs, in.Obs)
	}
	if !in.Missover {
		t.Error("Missover = false, want true")
	}
}

func TestParseFileAndPut(t *testing.T) {
	body := dataBody(t, `data _null_; set s; file "out.csv" dsd dlm=","; put "row" name age dollar8.2; run;`)

	fs, ok := body[1].(*ast.FileStatement)
	if !ok {
		t.Fatalf("stmt 1 is %T, want *ast.FileStatement", body[1])
	}
	if fs.Path != "out.csv" || !fs.DSD || fs.Delimiter != "," {
		t.Errorf("FileStatement = %+v, want path=out.csv dsd dlm=,", fs)
	}

	put, ok := body[2].(*ast.PutStatement)
	if !ok {
		t.Fatalf("stmt 2 is %T, want *ast.PutStatement", body[2])
	}
	if len(put.Items) != 3 {
		t.Fatalf("got %d items, want 3: %+v", len(put.Items), put.Items)
	}
	if !put.Items[0].IsLiteral || put.Items[0].Literal != "row" {
		t.Errorf("item 0 = %+v, want literal \"row\"", put.Items[0])
	}
	if put.Items[1].IsLiteral || put.Items[1].Var != "name" {
		t.Errorf("item 1 = %+v, want var name", put.Items[1])
	}
	// The trailing format binds to the most recent variable (age), not name.
	if put.Items[2].Var != "age" || put.Items[2].Format != "dollar8.2" {
		t.Errorf("item 2 = %+v, want var age format dollar8.2", put.Items[2])
	}
}

func TestParsePutNamedAndAll(t *testing.T) {
	body := dataBody(t, "data _null_; put id= name= _all_; run;")
	put := body[0].(*ast.PutStatement)
	if len(put.Items) != 3 {
		t.Fatalf("items = %d, want 3: %+v", len(put.Items), put.Items)
	}
	if !put.Items[0].Named || put.Items[0].Var != "id" {
		t.Errorf("item0 = %+v, want Named id", put.Items[0])
	}
	if !put.Items[1].Named || put.Items[1].Var != "name" {
		t.Errorf("item1 = %+v, want Named name", put.Items[1])
	}
	if !put.Items[2].AllVars {
		t.Errorf("item2 = %+v, want AllVars", put.Items[2])
	}
}

func TestParsePutNamedSpaced(t *testing.T) {
	body := dataBody(t, "data _null_; put x =; run;")
	put := body[0].(*ast.PutStatement)
	if len(put.Items) != 1 || !put.Items[0].Named || put.Items[0].Var != "x" {
		t.Fatalf("items = %+v, want one Named x", put.Items)
	}
}

func TestParseTimeDatetimeLiterals(t *testing.T) {
	body := dataBody(t, "data d; t = '14:30:00't; dt = '01JAN1960:00:00:01'dt; run;")
	tm, ok := body[0].(*ast.AssignmentStatement)
	if !ok {
		t.Fatalf("stmt0 is %T", body[0])
	}
	tn, ok := tm.Value.(*ast.NumberLiteral)
	if !ok || tn.Value != 52200 {
		t.Errorf("time literal = %+v, want 52200", tm.Value)
	}
	dm := body[1].(*ast.AssignmentStatement)
	dn, ok := dm.Value.(*ast.NumberLiteral)
	if !ok || dn.Value != 1 { // 1 second past the SAS datetime epoch
		t.Errorf("datetime literal = %+v, want 1", dm.Value)
	}
}

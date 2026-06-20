package sas7bdat

import (
	"encoding/csv"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/solifugus/ass/table"
)

func TestReadAirline(t *testing.T) {
	ds, err := Read("testdata/airline.sas7bdat")
	if err != nil {
		t.Fatal(err)
	}
	checkAgainstCSV(t, ds, "testdata/airline.csv")
}

func TestReadProductsales(t *testing.T) {
	ds, err := Read("testdata/productsales.sas7bdat")
	if err != nil {
		t.Fatal(err)
	}
	checkAgainstCSV(t, ds, "testdata/productsales.csv")
}

// The test1/test2/test3 trio is the same wide, mixed-type table (100 columns ×
// 10 rows) stored three ways: uncompressed, RLE (SASYZCRL), and RDC (SASYZCR2).
// All three must decode to the same values, verified against the shared CSV. This
// is the core value check for row decompression.
func TestReadUncompressed(t *testing.T) {
	ds, err := Read("testdata/test1.sas7bdat")
	if err != nil {
		t.Fatal(err)
	}
	if ds.Columns[1].Kind != table.Character || ds.Columns[3].Format != "mmddyy10." {
		t.Errorf("unexpected metadata: col2=%v col4 fmt=%q", ds.Columns[1].Kind, ds.Columns[3].Format)
	}
	checkAgainstCSV(t, ds, "testdata/test123.csv")
}

func TestReadRLE(t *testing.T) {
	ds, err := Read("testdata/test2_rle.sas7bdat")
	if err != nil {
		t.Fatal(err)
	}
	if ds.NObs() != 10 {
		t.Fatalf("obs = %d, want 10", ds.NObs())
	}
	checkAgainstCSV(t, ds, "testdata/test123.csv")
}

func TestReadRDC(t *testing.T) {
	ds, err := Read("testdata/test3_rdc.sas7bdat")
	if err != nil {
		t.Fatal(err)
	}
	if ds.NObs() != 10 {
		t.Fatalf("obs = %d, want 10", ds.NObs())
	}
	checkAgainstCSV(t, ds, "testdata/test123.csv")
}

// 0x40controlbyte is a tiny RLE file built to exercise the 0x40 control byte
// (repeat-the-following-byte): one row of three 50-character string fields.
func TestReadRLEControlByte(t *testing.T) {
	ds, err := Read("testdata/0x40controlbyte.sas7bdat")
	if err != nil {
		t.Fatal(err)
	}
	checkAgainstCSV(t, ds, "testdata/0x40controlbyte.csv")
}

// datetime exercises SAS date/datetime values (stored as numeric days/seconds
// from 1960-01-01). The CSV renders them as strings, so assert the underlying
// numbers directly against hand-derived values.
func TestReadDatetime(t *testing.T) {
	ds, err := Read("testdata/datetime.sas7bdat")
	if err != nil {
		t.Fatal(err)
	}
	if ds.NObs() != 4 {
		t.Fatalf("obs = %d, want 4", ds.NObs())
	}
	// Row index 1 is 1960-01-01 → SAS date 0 and datetime 0.
	if v := ds.Get(ds.Rows[1], "Date1"); v.Num != 0 {
		t.Errorf("Date1[1] = %v, want 0 (1960-01-01)", v.Num)
	}
	if v := ds.Get(ds.Rows[1], "DateTime"); v.Num != 0 {
		t.Errorf("DateTime[1] = %v, want 0", v.Num)
	}
	// Row 0 is 1677-09-22 → -103098 days from the SAS epoch.
	if v := ds.Get(ds.Rows[0], "Date1"); v.Num != -103098 {
		t.Errorf("Date1[0] = %v, want -103098", v.Num)
	}
	if ds.Columns[0].Format != "yymmdd10." || ds.Columns[2].Format != "datetime19." {
		t.Errorf("date formats: %q %q", ds.Columns[0].Format, ds.Columns[2].Format)
	}
}

// checkAgainstCSV verifies the dataset's columns and values against a paired CSV
// reference (the pandas test corpus ships one alongside each .sas7bdat). Numeric
// columns compare within a relative tolerance (some values are stored truncated);
// character columns compare exactly after trimming.
func checkAgainstCSV(t *testing.T, ds *table.Dataset, csvPath string) {
	t.Helper()
	f, err := os.Open(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cr := csv.NewReader(f)
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	head, body := rows[0], rows[1:]

	if len(ds.Columns) != len(head) {
		t.Fatalf("column count = %d, want %d (%v vs %v)", len(ds.Columns), len(head), ds.ColumnNames(), head)
	}
	for i, name := range head {
		if !strings.EqualFold(ds.Columns[i].Name, name) {
			t.Errorf("column %d = %q, want %q", i, ds.Columns[i].Name, name)
		}
	}
	if ds.NObs() != len(body) {
		t.Fatalf("rows = %d, want %d", ds.NObs(), len(body))
	}
	for ri, want := range body {
		got := ds.Rows[ri]
		for ci, col := range ds.Columns {
			v := ds.Get(got, col.Name)
			cell := want[ci]
			if col.Kind == table.Character {
				if v.Str != cell {
					t.Errorf("row %d col %s: %q, want %q", ri, col.Name, v.Str, cell)
				}
				continue
			}
			// Numeric: the CSV may render a date column as YYYY-MM-DD; skip those
			// here (date handling is verified separately).
			wf, err := strconv.ParseFloat(cell, 64)
			if err != nil {
				continue
			}
			if v.IsMissing() {
				if cell != "" {
					t.Errorf("row %d col %s: missing, want %v", ri, col.Name, wf)
				}
				continue
			}
			tol := math.Abs(wf) * 1e-6
			if tol < 1e-9 {
				tol = 1e-9
			}
			if math.Abs(v.Num-wf) > tol {
				t.Errorf("row %d col %s: %.12g, want %.12g", ri, col.Name, v.Num, wf)
			}
		}
	}
}

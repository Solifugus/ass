// Package xlsx is a minimal, dependency-free reader and writer for the subset of
// the Office Open XML (.xlsx) spreadsheet format that PROC IMPORT/EXPORT needs:
// a single worksheet of string/number cells with a header row. It uses only the
// Go standard library (archive/zip + encoding/xml).
//
// Writing inlines string cells (t="inlineStr") so no shared-strings table is
// needed. Reading resolves shared strings when present and uses each cell's A1
// reference to place values in the right column (filling gaps with "").
package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Read returns the rows of the first worksheet in the .xlsx file at path, each
// row a slice of cell strings (numbers rendered as their stored text). Rows are
// padded to the width of the widest row.
func Read(path string) ([][]string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var sheetData, sharedData []byte
	sheetName := ""
	for _, f := range zr.File {
		switch {
		case strings.HasPrefix(f.Name, "xl/worksheets/sheet") && strings.HasSuffix(f.Name, ".xml"):
			// Pick the lexically-first sheet file (sheet1.xml).
			if sheetName == "" || f.Name < sheetName {
				if b, err := readZip(f); err == nil {
					sheetName, sheetData = f.Name, b
				}
			}
		case f.Name == "xl/sharedStrings.xml":
			if b, err := readZip(f); err == nil {
				sharedData = b
			}
		}
	}
	if sheetData == nil {
		return nil, fmt.Errorf("xlsx: no worksheet found in %s", path)
	}

	shared, err := parseSharedStrings(sharedData)
	if err != nil {
		return nil, err
	}
	return parseSheet(sheetData, shared)
}

func readZip(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(rc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// parseSharedStrings reads the sharedStrings.xml <si> entries (each may hold a
// plain <t> or several <r><t> runs).
func parseSharedStrings(data []byte) ([]string, error) {
	if len(data) == 0 {
		return nil, nil
	}
	type tNode struct {
		Text string `xml:",chardata"`
	}
	type si struct {
		T  tNode   `xml:"t"`
		RT []tNode `xml:"r>t"`
	}
	type sst struct {
		SI []si `xml:"si"`
	}
	var s sst
	if err := xml.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	out := make([]string, len(s.SI))
	for i, e := range s.SI {
		if len(e.RT) > 0 {
			var b strings.Builder
			for _, r := range e.RT {
				b.WriteString(r.Text)
			}
			out[i] = b.String()
		} else {
			out[i] = e.T.Text
		}
	}
	return out, nil
}

// parseSheet reads worksheet rows, resolving each cell's value by its type and
// placing it at the column index parsed from its A1 reference.
func parseSheet(data []byte, shared []string) ([][]string, error) {
	type cell struct {
		R  string `xml:"r,attr"` // e.g. "B3"
		T  string `xml:"t,attr"` // cell type: s, inlineStr, str, b, ...
		V  string `xml:"v"`      // value (or shared-string index)
		IS struct {
			T  string   `xml:"t"`
			RT []string `xml:"r>t"`
		} `xml:"is"` // inline string
	}
	type row struct {
		Cells []cell `xml:"c"`
	}
	type sheet struct {
		Rows []row `xml:"sheetData>row"`
	}
	var sh sheet
	if err := xml.Unmarshal(data, &sh); err != nil {
		return nil, err
	}

	var out [][]string
	width := 0
	for _, r := range sh.Rows {
		cells := map[int]string{}
		maxc := -1
		for i, c := range r.Cells {
			col := colIndex(c.R, i)
			var val string
			switch c.T {
			case "s": // shared string: V is the index
				if idx, err := strconv.Atoi(strings.TrimSpace(c.V)); err == nil && idx >= 0 && idx < len(shared) {
					val = shared[idx]
				}
			case "inlineStr":
				if len(c.IS.RT) > 0 {
					val = strings.Join(c.IS.RT, "")
				} else {
					val = c.IS.T
				}
			default: // "str", "n", "b", or empty: the literal value
				val = c.V
			}
			cells[col] = val
			if col > maxc {
				maxc = col
			}
		}
		line := make([]string, maxc+1)
		for c, v := range cells {
			line[c] = v
		}
		out = append(out, line)
		if len(line) > width {
			width = len(line)
		}
	}
	// Pad every row to the widest width.
	for i := range out {
		for len(out[i]) < width {
			out[i] = append(out[i], "")
		}
	}
	return out, nil
}

// colIndex returns the 0-based column from an A1 cell reference (the leading
// letters); if the reference is absent it falls back to the positional index.
func colIndex(ref string, fallback int) int {
	letters := ""
	for _, ch := range ref {
		if ch >= 'A' && ch <= 'Z' {
			letters += string(ch)
		} else if ch >= 'a' && ch <= 'z' {
			letters += string(ch - 32)
		} else {
			break
		}
	}
	if letters == "" {
		return fallback
	}
	n := 0
	for _, ch := range letters {
		n = n*26 + int(ch-'A'+1)
	}
	return n - 1
}

// colRef returns the A1 column letters for a 0-based column index.
func colRef(col int) string {
	col++
	s := ""
	for col > 0 {
		col--
		s = string(rune('A'+col%26)) + s
		col /= 26
	}
	return s
}

// Write creates a single-worksheet .xlsx file at path. Each row is written as a
// row of cells; a cell whose value parses as a number is written as a numeric
// cell, otherwise as an inline string. numericMask[c]==true forces column c to be
// written as text even when it looks numeric (so header rows stay strings).
func Write(path string, rows [][]string, numericCol func(rowIdx, col int) bool) error {
	var sheet bytes.Buffer
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for ri, r := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, ri+1)
		for ci, v := range r {
			ref := colRef(ci) + strconv.Itoa(ri+1)
			if numericCol != nil && numericCol(ri, ci) && isNumber(v) {
				fmt.Fprintf(&sheet, `<c r="%s"><v>%s</v></c>`, ref, v)
			} else {
				fmt.Fprintf(&sheet, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, ref, xmlEscape(v))
			}
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
			`</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
			`</Relationships>`,
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
			`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
			`</Relationships>`,
		"xl/worksheets/sheet1.xml": sheet.String(),
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(content)); err != nil {
			return err
		}
	}
	return zw.Close()
}

func isNumber(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	_, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return err == nil
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

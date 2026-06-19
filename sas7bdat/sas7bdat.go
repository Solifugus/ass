// Package sas7bdat reads SAS dataset files (.sas7bdat) into table.Dataset
// values. It is a clean-room implementation built from the public reverse-
// engineering literature on the format (the layout documented by the ReadStat
// and sas7bdat open-source projects and Matthew Shotwell's published format
// notes) — never from proprietary SAS documentation, source, or internals.
//
// Scope is read-only and eager (the whole file is materialized into an in-memory
// table.Dataset), matching the existing LIBNAME-engine model in package dbio.
package sas7bdat

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/solifugus/ass/table"
)

// magic is the 32-byte signature every .sas7bdat file begins with.
var magic = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0xc2, 0xea, 0x81, 0x60,
	0xb3, 0x14, 0x11, 0xcf, 0xbd, 0x92, 0x08, 0x00,
	0x09, 0xc7, 0x31, 0x8c, 0x18, 0x1f, 0x10, 0x11,
}

// header holds the parsed file-header fields needed to walk the pages.
type header struct {
	u64          bool // 64-bit alignment (wider integer fields and pointers)
	littleEndian bool
	headerLength int
	pageLength   int
	pageCount    int
	name         string

	bo binary.ByteOrder
}

// reader carries the file bytes plus the parsed header while decoding.
type reader struct {
	data []byte
	hdr  header
}

// parseHeader reads and validates the file header.
func (r *reader) parseHeader() error {
	d := r.data
	if len(d) < 1024 {
		return fmt.Errorf("file too short to be sas7bdat (%d bytes)", len(d))
	}
	for i := range magic {
		if d[i] != magic[i] {
			return fmt.Errorf("not a sas7bdat file (bad magic number)")
		}
	}
	h := &r.hdr

	// Alignment flags. Byte 32 == '3' marks 64-bit layout (wider integer fields
	// and an 8-byte page count). Byte 35 == '3' shifts the header sizing block by
	// 4 bytes (align1).
	align1 := 0
	if d[32] == '3' {
		h.u64 = true
	}
	if d[35] == '3' {
		align1 = 4
	}

	h.littleEndian = d[37] == 0x01
	if h.littleEndian {
		h.bo = binary.LittleEndian
	} else {
		h.bo = binary.BigEndian
	}

	// Dataset name: 64 bytes at offset 92, space-padded.
	h.name = trimName(d[92:156])

	// The page/header sizing block shifts by align1.
	h.headerLength = int(r.u32(196 + align1))
	h.pageLength = int(r.u32(200 + align1))
	if h.u64 {
		h.pageCount = int(r.u64v(204 + align1))
	} else {
		h.pageCount = int(r.u32(204 + align1))
	}

	if h.headerLength <= 0 || h.pageLength <= 0 {
		return fmt.Errorf("invalid header sizing (header=%d page=%d)", h.headerLength, h.pageLength)
	}
	return nil
}

// --- little/big-endian field readers (honor parsed byte order) ---

func (r *reader) u16(off int) uint16  { return r.hdr.bo.Uint16(r.data[off : off+2]) }
func (r *reader) u32(off int) uint32  { return r.hdr.bo.Uint32(r.data[off : off+4]) }
func (r *reader) u64v(off int) uint64 { return r.hdr.bo.Uint64(r.data[off : off+8]) }

// f64 reads a SAS double. SAS may store numerics truncated to fewer than 8
// bytes (trailing zero bytes dropped); we left-pad/right-pad to 8 by byte order.
func (r *reader) f64(off, length int) float64 {
	var b [8]byte
	if r.hdr.littleEndian {
		// Value occupies the high-order bytes; missing low bytes are zero.
		copy(b[8-length:], r.data[off:off+length])
	} else {
		copy(b[:length], r.data[off:off+length])
	}
	return math.Float64frombits(r.hdr.bo.Uint64(b[:]))
}

// --- page / subheader geometry (depends on u64) ---

func (r *reader) intSize() int {
	if r.hdr.u64 {
		return 8
	}
	return 4
}

func (r *reader) pageBitOffset() int {
	if r.hdr.u64 {
		return 32
	}
	return 16
}

func (r *reader) subheaderPtrLen() int {
	if r.hdr.u64 {
		return 24
	}
	return 12
}

func (r *reader) pageStart(p int) int { return r.hdr.headerLength + p*r.hdr.pageLength }

// readInt reads an intSize-wide little/big-endian integer.
func (r *reader) readInt(off int) int {
	if r.hdr.u64 {
		return int(r.u64v(off))
	}
	return int(r.u32(off))
}

// signature reads the subheader signature (intSize-wide) at an absolute offset.
func (r *reader) signature(off int) uint64 {
	if r.hdr.u64 {
		return r.u64v(off)
	}
	return uint64(r.u32(off))
}

type subheaderPtr struct {
	offset      int // relative to page start
	length      int
	compression byte
	typ         byte
}

func (r *reader) subheaderPtr(pageStart, i int) subheaderPtr {
	is := r.intSize()
	base := pageStart + r.pageBitOffset() + 8 + i*r.subheaderPtrLen()
	return subheaderPtr{
		offset:      r.readInt(base),
		length:      r.readInt(base + is),
		compression: r.data[base+2*is],
		typ:         r.data[base+2*is+1],
	}
}

func trimName(b []byte) string {
	end := len(b)
	for end > 0 && (b[end-1] == ' ' || b[end-1] == 0x00) {
		end--
	}
	return string(b[:end])
}

// readFileBytes loads a file fully into memory.
func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// --- subheader signatures (x86 32-bit forms; the u64 forms are the same low
// bytes widened to 8, handled by sigKind via masking) ---

const (
	sigRowSize     = 0xF7F7F7F7
	sigColSize     = 0xF6F6F6F6
	sigColText     = 0xFFFFFFFD
	sigColName     = 0xFFFFFFFF
	sigColAttr     = 0xFFFFFFFC
	sigColFmtLabel = 0xFFFFFBFE
	sigColList     = 0xFFFFFFFE
)

type subKind int

const (
	subUnknown subKind = iota
	subRowSize
	subColSize
	subColText
	subColName
	subColAttr
	subColFmtLabel
)

// sigKind classifies a subheader signature, normalizing the 64-bit forms (which
// repeat the same byte pattern in the high word, e.g. 0xF7F7F7F7F7F7F7F7) to the
// 32-bit classification.
func sigKind(sig uint64) subKind {
	low := uint32(sig)
	switch low {
	case sigRowSize:
		return subRowSize
	case sigColSize:
		return subColSize
	case sigColText:
		return subColText
	case sigColName:
		return subColName
	case sigColAttr:
		return subColAttr
	case sigColFmtLabel:
		return subColFmtLabel
	}
	return subUnknown
}

// compressionMethod returns the SAS row-compression signature found in a
// column-text block ("SASYZCRL" = RLE, "SASYZCR2" = RDC), or "" if the data is
// uncompressed. Decompression is not yet implemented, so a non-empty result
// means the file cannot be read.
func compressionMethod(block []byte) string {
	for _, m := range []string{"SASYZCRL", "SASYZCR2"} {
		if bytes.Contains(block, []byte(m)) {
			return m
		}
	}
	return ""
}

// column is the decoded metadata for one variable.
type column struct {
	name    string
	offset  int // byte offset within a row
	length  int // stored byte width
	numeric bool
	format  string
	label   string
}

// page type classification. Data rows live on DATA and MIX pages; metadata
// subheaders live on META and MIX pages.
func isMixType(t uint16) bool  { return t == 512 || t == 640 }
func isDataType(t uint16) bool { return t == 256 }

// parse walks the file and returns the decoded column metadata, row geometry,
// and accumulated text/name/attr/format records.
func (r *reader) parse() (*table.Dataset, error) {
	if err := r.parseHeader(); err != nil {
		return nil, err
	}

	var (
		rowLength, rowCount, mixRowCount int
		textBlocks                       [][]byte
		names                            []nameRef
		attrs                            []attrRec
		fmtLabels                        []fmtRef
	)

	bo := r.pageBitOffset()
	is := r.intSize()

	// Pass 1: metadata from subheaders on META and MIX pages.
	for p := 0; p < r.hdr.pageCount; p++ {
		ps := r.pageStart(p)
		pageType := r.u16(ps + bo)
		subCount := int(r.u16(ps + bo + 4))
		if isDataType(pageType) {
			continue // pure data page carries no subheaders
		}
		for i := 0; i < subCount; i++ {
			sp := r.subheaderPtr(ps, i)
			if sp.length == 0 || sp.compression == 1 { // truncated/empty
				continue
			}
			off := ps + sp.offset
			switch sigKind(r.signature(off)) {
			case subRowSize:
				rowLength = r.readInt(off + 5*is)
				rowCount = r.readInt(off + 6*is)
				mixRowCount = r.readInt(off + 15*is)
			case subColText:
				block := r.data[off+is : off+sp.length]
				if c := compressionMethod(block); c != "" {
					return nil, fmt.Errorf("compressed sas7bdat (%s) is not yet supported", c)
				}
				textBlocks = append(textBlocks, block)
			case subColName:
				names = append(names, r.readNameRefs(off, sp.length)...)
			case subColAttr:
				attrs = append(attrs, r.readAttrRecs(off, sp.length)...)
			case subColFmtLabel:
				fmtLabels = append(fmtLabels, r.readFmtRef(off))
			}
		}
	}

	if rowLength == 0 {
		return nil, fmt.Errorf("missing ROW_SIZE subheader")
	}

	// Assemble columns from the parallel name/attr/format records.
	cols := make([]column, len(attrs))
	for i := range attrs {
		c := column{offset: attrs[i].offset, length: attrs[i].length, numeric: attrs[i].numeric}
		if i < len(names) {
			c.name = refText(textBlocks, names[i].textIdx, names[i].offset, names[i].length)
		}
		if i < len(fmtLabels) {
			fr := fmtLabels[i]
			c.format = sasFormat(refText(textBlocks, fr.fmtIdx, fr.fmtOff, fr.fmtLen), fr.width, fr.ndec)
			c.label = refText(textBlocks, fr.lblIdx, fr.lblOff, fr.lblLen)
		}
		cols[i] = c
	}

	ds := table.NewDataset("", r.hdr.name)
	for _, c := range cols {
		col := table.Column{Name: c.name, Length: c.length, Format: c.format}
		col.Label = c.label
		if c.numeric {
			col.Kind = table.Numeric
		} else {
			col.Kind = table.Character
		}
		ds.AddColumn(col)
	}

	// Pass 2: extract rows from DATA and MIX pages.
	read := 0
	for p := 0; p < r.hdr.pageCount && read < rowCount; p++ {
		ps := r.pageStart(p)
		pageType := r.u16(ps + bo)
		blockCount := int(r.u16(ps + bo + 2))
		subCount := int(r.u16(ps + bo + 4))

		// Row data follows the subheader-pointer array, padded up to the next
		// 8-byte boundary.
		base := bo + 8 + subCount*r.subheaderPtrLen()
		dataStart := base + (base % 8)

		var rowsHere int
		switch {
		case isDataType(pageType):
			rowsHere = blockCount
		case isMixType(pageType):
			rowsHere = mixRowCount
		default:
			continue
		}
		if rowsHere > rowCount-read {
			rowsHere = rowCount - read
		}
		for k := 0; k < rowsHere; k++ {
			rowOff := ps + dataStart + k*rowLength
			ds.AppendRow(r.decodeRow(cols, rowOff))
			read++
		}
	}
	return ds, nil
}

type nameRef struct{ textIdx, offset, length int }
type attrRec struct {
	offset, length int
	numeric        bool
}
type fmtRef struct {
	width, ndec            int
	fmtIdx, fmtOff, fmtLen int
	lblIdx, lblOff, lblLen int
}

// readNameRefs decodes the column-name pointer array. Pointers begin at
// off+intSize+8, each 8 bytes: textIndex(u16), offset(u16), length(u16), pad.
func (r *reader) readNameRefs(off, length int) []nameRef {
	is := r.intSize()
	count := (length - 2*is - 12) / 8
	base := off + is + 8
	out := make([]nameRef, 0, count)
	for i := 0; i < count; i++ {
		b := base + i*8
		out = append(out, nameRef{
			textIdx: int(r.u16(b)),
			offset:  int(r.u16(b + 2)),
			length:  int(r.u16(b + 4)),
		})
	}
	return out
}

// readAttrRecs decodes the column-attribute vectors. Vectors begin at
// off+intSize+8, stride intSize+8: dataOffset(intSize), dataLength(u32 @ +intSize),
// type byte(@ +intSize+6; 1=numeric, 2=character).
func (r *reader) readAttrRecs(off, length int) []attrRec {
	is := r.intSize()
	vlen := is + 8
	count := (length - 2*is - 8) / vlen
	base := off + is + 8
	out := make([]attrRec, 0, count)
	for i := 0; i < count; i++ {
		b := base + i*vlen
		out = append(out, attrRec{
			offset:  r.readInt(b),
			length:  int(r.u32(b + is)),
			numeric: r.data[b+is+6] == 1,
		})
	}
	return out
}

// readFmtRef decodes one format/label subheader: the format width/decimals plus
// the (textIndex, offset, length) references into the column-text blocks for the
// format name and the column label. The field offsets below are verified for
// 32-bit files; on 64-bit files the leading integer block is wider, so
// format/label recovery is deferred (the values still read correctly — only the
// display format/label metadata is skipped).
func (r *reader) readFmtRef(off int) fmtRef {
	if r.hdr.u64 {
		return fmtRef{}
	}
	return fmtRef{
		width: int(r.u16(off + 12)), ndec: int(r.u16(off + 14)),
		fmtIdx: int(r.u16(off + 34)), fmtOff: int(r.u16(off + 36)), fmtLen: int(r.u16(off + 38)),
		lblIdx: int(r.u16(off + 40)), lblOff: int(r.u16(off + 42)), lblLen: int(r.u16(off + 44)),
	}
}

// refText extracts a (textIdx, offset, length) reference from the collected text
// blocks, trimming trailing spaces/NULs.
func refText(blocks [][]byte, idx, off, length int) string {
	if length <= 0 || idx < 0 || idx >= len(blocks) {
		return ""
	}
	b := blocks[idx]
	if off < 0 || off+length > len(b) {
		return ""
	}
	return strings.TrimRight(string(b[off:off+length]), " \x00")
}

// decodeRow reads one observation at the given absolute offset.
func (r *reader) decodeRow(cols []column, rowOff int) table.Row {
	row := make(table.Row, len(cols))
	for _, c := range cols {
		key := strings.ToLower(c.name)
		if c.numeric {
			f := r.f64(rowOff+c.offset, c.length)
			if math.IsNaN(f) {
				row[key] = table.MissingNum()
			} else {
				row[key] = table.Num(f)
			}
		} else {
			s := strings.TrimRight(string(r.data[rowOff+c.offset:rowOff+c.offset+c.length]), " \x00")
			row[key] = table.Char(s)
		}
	}
	return row
}

// sasFormat reconstructs a SAS format spec (name + width[.decimals] + ".") from
// the stored name and width/decimals, e.g. ("DOLLAR",12,2) -> "dollar12.2",
// ("BEST",12,0) -> "best12.", ("$CHAR",10,0) -> "$char10.".
func sasFormat(name string, width, ndec int) string {
	if name == "" {
		return ""
	}
	name = strings.ToLower(name)
	switch {
	case width <= 0:
		return name + "."
	case ndec > 0:
		return fmt.Sprintf("%s%d.%d", name, width, ndec)
	default:
		return fmt.Sprintf("%s%d.", name, width)
	}
}

// Read opens a .sas7bdat file and returns it as an in-memory dataset.
func Read(path string) (*table.Dataset, error) {
	data, err := readFileBytes(path)
	if err != nil {
		return nil, err
	}
	r := &reader{data: data}
	ds, err := r.parse()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return ds, nil
}

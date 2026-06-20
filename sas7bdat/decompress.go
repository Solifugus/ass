package sas7bdat

import "fmt"

// Row decompression for the two SAS-native row-compression schemes:
//
//   - SASYZCRL — Run Length Encoding (RLE).
//   - SASYZCR2 — Ross Data Compression (RDC), a LZ-style scheme with
//     back-references into the already-decompressed output.
//
// Both are clean-room ports of the public reverse-engineering literature on the
// format (the RLE command table documented by the ReadStat project and the
// sas7bdat R-package vignette; RDC as described by Ed Ross's published algorithm
// in Dr. Dobb's Journal, 1992) — never from proprietary SAS material. In a
// compressed file each observation is stored as its own row subheader; a row
// whose stored length is already the full row width was not worth compressing
// and is used verbatim, otherwise it expands to exactly rowLength bytes.

// rleDecompress expands a SAS RLE (SASYZCRL) row to resultLen bytes. The control
// byte's high nibble selects a command and its low nibble (end-of-first-byte)
// contributes to the run length; copy commands take literal bytes from the
// input, insert commands repeat a single byte (a following byte, a space 0x20, a
// NUL 0x00, or an '@' 0x40).
func rleDecompress(in []byte, resultLen int) ([]byte, error) {
	out := make([]byte, 0, resultLen)
	ip := 0
	// copyRun appends n literal bytes from the input stream.
	copyRun := func(n int) error {
		if ip+n > len(in) {
			return fmt.Errorf("sas7bdat RLE: literal run overruns input")
		}
		out = append(out, in[ip:ip+n]...)
		ip += n
		return nil
	}
	// fill appends n copies of b.
	fill := func(b byte, n int) {
		for i := 0; i < n; i++ {
			out = append(out, b)
		}
	}
	for ip < len(in) {
		control := in[ip] & 0xF0
		eofb := int(in[ip] & 0x0F)
		ip++
		switch control {
		case 0x00: // copy (next + 64 + eofb*256) literal bytes
			n := int(in[ip]) + 64 + eofb*256
			ip++
			if err := copyRun(n); err != nil {
				return nil, err
			}
		case 0x40: // repeat the following byte (next + 18 + eofb*256) times
			n := int(in[ip]) + 18 + eofb*256
			ip++
			fill(in[ip], n)
			ip++
		case 0x60: // (eofb*256 + next + 17) spaces
			n := eofb*256 + int(in[ip]) + 17
			ip++
			fill(0x20, n)
		case 0x70: // (eofb*256 + next + 17) NULs
			n := eofb*256 + int(in[ip]) + 17
			ip++
			fill(0x00, n)
		case 0x80: // copy (eofb + 1) literal bytes
			if err := copyRun(eofb + 1); err != nil {
				return nil, err
			}
		case 0x90: // copy (eofb + 17) literal bytes
			if err := copyRun(eofb + 17); err != nil {
				return nil, err
			}
		case 0xA0: // copy (eofb + 33) literal bytes
			if err := copyRun(eofb + 33); err != nil {
				return nil, err
			}
		case 0xB0: // copy (eofb + 49) literal bytes
			if err := copyRun(eofb + 49); err != nil {
				return nil, err
			}
		case 0xC0: // repeat the following byte (eofb + 3) times
			b := in[ip]
			ip++
			fill(b, eofb+3)
		case 0xD0: // (eofb + 2) '@' bytes
			fill(0x40, eofb+2)
		case 0xE0: // (eofb + 2) spaces
			fill(0x20, eofb+2)
		case 0xF0: // (eofb + 2) NULs
			fill(0x00, eofb+2)
		default:
			return nil, fmt.Errorf("sas7bdat RLE: unknown control byte 0x%02x", control)
		}
	}
	return out, nil
}

// rdcDecompress expands a SAS RDC (SASYZCR2) row to resultLen bytes. A 16-bit
// control word is consumed two bytes at a time; each control bit, MSB first,
// selects between a literal byte (bit 0) and a command token (bit 1). Command
// tokens are short/long runs of a repeated byte or back-references that copy a
// run from earlier in the decompressed output.
func rdcDecompress(in []byte, resultLen int) ([]byte, error) {
	out := make([]byte, 0, resultLen)
	var ctrlBits, ctrlMask uint16
	ip := 0
	for ip < len(in) {
		ctrlMask >>= 1
		if ctrlMask == 0 {
			if ip+1 >= len(in) {
				break
			}
			ctrlBits = uint16(in[ip])<<8 + uint16(in[ip+1])
			ip += 2
			ctrlMask = 0x8000
		}
		if ip >= len(in) {
			break
		}
		if ctrlBits&ctrlMask == 0 { // literal byte
			out = append(out, in[ip])
			ip++
			continue
		}
		cmd := (in[ip] >> 4) & 0x0F
		cnt := int(in[ip] & 0x0F)
		ip++
		switch {
		case cmd == 0: // short run of a repeated byte
			cnt += 3
			b := in[ip]
			ip++
			for k := 0; k < cnt; k++ {
				out = append(out, b)
			}
		case cmd == 1: // long run of a repeated byte
			cnt += int(in[ip])<<4 + 19
			ip++
			b := in[ip]
			ip++
			for k := 0; k < cnt; k++ {
				out = append(out, b)
			}
		case cmd == 2: // long back-reference
			ofs := cnt + 3 + int(in[ip])<<4
			ip++
			cnt = int(in[ip]) + 16
			ip++
			start := len(out) - ofs
			if start < 0 {
				return nil, fmt.Errorf("sas7bdat RDC: back-reference before start of output")
			}
			for k := 0; k < cnt; k++ {
				out = append(out, out[start+k])
			}
		default: // short back-reference (cmd 3..15: cmd is the run length)
			ofs := cnt + 3 + int(in[ip])<<4
			ip++
			start := len(out) - ofs
			if start < 0 {
				return nil, fmt.Errorf("sas7bdat RDC: back-reference before start of output")
			}
			for k := 0; k < int(cmd); k++ {
				out = append(out, out[start+k])
			}
		}
	}
	return out, nil
}

// decompressRow expands a single stored row to rowLength bytes using the file's
// compression method. A row already at full width was stored uncompressed.
func decompressRow(method string, stored []byte, rowLength int) ([]byte, error) {
	if len(stored) >= rowLength {
		return stored, nil
	}
	var (
		out []byte
		err error
	)
	switch method {
	case "SASYZCRL":
		out, err = rleDecompress(stored, rowLength)
	case "SASYZCR2":
		out, err = rdcDecompress(stored, rowLength)
	default:
		return nil, fmt.Errorf("sas7bdat: unknown compression method %q", method)
	}
	if err != nil {
		return nil, err
	}
	if len(out) < rowLength {
		return nil, fmt.Errorf("sas7bdat: decompressed row is %d bytes, expected %d", len(out), rowLength)
	}
	return out[:rowLength], nil
}

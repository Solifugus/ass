// Package lexer tokenizes SAS source text into a stream of tokens for the
// parser. It tracks line/column positions and handles SAS-specific lexical
// quirks such as the two comment forms, the $ character-variable marker, and
// the raw datalines region.
package lexer

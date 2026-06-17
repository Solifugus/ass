package lexer

import "strings"

// TokenType identifies the lexical class of a Token. It is a string so that
// test tables and debug output are human-readable.
type TokenType string

// Token is a single lexical unit produced by the Lexer, with source position.
// Line and Col are 1-based; Col is the column of the token's first rune.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Col     int
}

const (
	// Special
	ILLEGAL TokenType = "ILLEGAL" // an unrecognized rune; Literal holds it
	EOF     TokenType = "EOF"     // end of input

	// Literals
	IDENT  TokenType = "IDENT"  // identifiers and unreserved words (e.g. age, name, set)
	NUMBER TokenType = "NUMBER" // numeric literal (123, 4.5, 1e3)
	STRING TokenType = "STRING" // quoted string; Literal is the unquoted value

	// Structural keywords (recognized by the lexer because they affect
	// step boundaries and, for datalines/cards, lexing mode). All other SAS
	// words are returned as IDENT and classified by the parser.
	DATA      TokenType = "DATA"
	PROC      TokenType = "PROC"
	RUN       TokenType = "RUN"
	QUIT      TokenType = "QUIT"
	DATALINES TokenType = "DATALINES" // datalines / cards keyword
	// DATALINES_DATA carries the raw inline data block (see lexer mode in 2.4),
	// with its lines preserved verbatim and joined by "\n".
	DATALINES_DATA TokenType = "DATALINES_DATA"

	// Operators
	EQ     TokenType = "EQ"     // =  (assignment or equality; parser decides)
	NE     TokenType = "NE"     // ^= ~= <>
	LT     TokenType = "LT"     // <
	LE     TokenType = "LE"     // <=
	GT     TokenType = "GT"     // >
	GE     TokenType = "GE"     // >=
	PLUS   TokenType = "PLUS"   // +
	MINUS  TokenType = "MINUS"  // -
	STAR   TokenType = "STAR"   // *
	SLASH  TokenType = "SLASH"  // /
	POW    TokenType = "POW"    // **
	CONCAT TokenType = "CONCAT" // ||
	AMP    TokenType = "AMP"    // &  (logical AND symbol / macro var prefix)
	PIPE   TokenType = "PIPE"   // |  (logical OR symbol)
	NOT    TokenType = "NOT"    // ^ ~ (logical NOT symbol)
	PERCENT TokenType = "PERCENT" // % (macro trigger)

	// Punctuation
	SEMICOLON TokenType = "SEMICOLON" // ;
	LPAREN    TokenType = "LPAREN"    // (
	RPAREN    TokenType = "RPAREN"    // )
	COMMA     TokenType = "COMMA"     // ,
	DOT       TokenType = "DOT"       // .
	DOLLAR    TokenType = "DOLLAR"    // $ (character-variable marker in INPUT, format prefix)
)

// keywords maps lowercased identifiers to their structural keyword TokenType.
// Words not present here are IDENT.
var keywords = map[string]TokenType{
	"data":       DATA,
	"proc":       PROC,
	"run":        RUN,
	"quit":       QUIT,
	"datalines":  DATALINES,
	"cards":      DATALINES,
	"datalines4": DATALINES,
	"cards4":     DATALINES,
}

// LookupIdent returns the keyword TokenType for an identifier if it is a
// structural keyword (case-insensitive, as SAS is), otherwise IDENT.
func LookupIdent(ident string) TokenType {
	if tt, ok := keywords[strings.ToLower(ident)]; ok {
		return tt
	}
	return IDENT
}

// IsDatalinesKeyword reports whether tt marks the start of an inline data block.
func IsDatalinesKeyword(tt TokenType) bool { return tt == DATALINES }

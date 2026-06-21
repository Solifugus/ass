//go:build !cgo

package corpus

import "regexp"

// In a pure-Go build (CGO_ENABLED=0), PROC SQL and the SQLite LIBNAME engine —
// both backed by the embedded SQLite (CGo) — are compiled out. Items that need
// them are skipped by the harness rather than reported as failures.

var sqliteLibname = regexp.MustCompile(`(?i)\blibname\s+\w+\s+sqlite\b`)

// itemNeedsUnavailable reports whether an item exercises PROC SQL or a SQLite
// LIBNAME engine, neither of which is available without CGo.
func itemNeedsUnavailable(it Item) bool {
	return it.HasFeature("proc-sql") || sqliteLibname.MatchString(it.Input)
}

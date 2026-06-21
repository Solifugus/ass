//go:build cgo

package corpus

// itemNeedsUnavailable reports whether an item exercises a feature that is
// compiled out of this build. In a CGo build, PROC SQL and the SQLite LIBNAME
// engine are present, so nothing is unavailable.
func itemNeedsUnavailable(Item) bool { return false }

package table

import "strings"

// Library is an in-memory collection of datasets keyed by name, modeling a SAS
// library such as WORK. Names are case-insensitive (stored uppercased), as in
// SAS. This is how steps pass data to one another.
type Library struct {
	datasets map[string]*Dataset
}

// NewLibrary creates an empty library.
func NewLibrary() *Library {
	return &Library{datasets: make(map[string]*Dataset)}
}

// Put stores (or replaces) a dataset under its name.
func (l *Library) Put(ds *Dataset) {
	l.datasets[strings.ToUpper(ds.Name)] = ds
}

// Get retrieves a dataset by name (case-insensitive). A name may be qualified
// as "lib.name"; the library component is currently ignored (everything lives
// in one in-memory library).
func (l *Library) Get(name string) (*Dataset, bool) {
	ds, ok := l.datasets[strings.ToUpper(datasetKey(name))]
	return ds, ok
}

// Has reports whether a dataset with the given name exists.
func (l *Library) Has(name string) bool {
	_, ok := l.Get(name)
	return ok
}

// Names returns the stored dataset names (uppercased), unordered.
func (l *Library) Names() []string {
	names := make([]string, 0, len(l.datasets))
	for k := range l.datasets {
		names = append(names, k)
	}
	return names
}

// datasetKey extracts the dataset component from a possibly-qualified name
// ("work.people" -> "people").
func datasetKey(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

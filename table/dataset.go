package table

import "strings"

// Column is the metadata for one variable in a dataset.
type Column struct {
	Name     string
	Kind     Kind
	Label    string
	Format   string
	Informat string
	Length   int
}

// Row is one observation: a mapping from (case-insensitive) variable name to
// value. Variable names are stored lowercased for lookup; Column preserves the
// display name and order.
type Row map[string]Value

// Dataset is a SAS dataset: ordered column metadata plus rows. Lib is the
// library (default "WORK"); Name is the dataset name.
type Dataset struct {
	Lib     string
	Name    string
	Columns []Column
	Rows    []Row
}

// NewDataset creates an empty dataset in the given library (defaulting to WORK).
func NewDataset(lib, name string) *Dataset {
	if lib == "" {
		lib = "WORK"
	}
	return &Dataset{Lib: strings.ToUpper(lib), Name: name}
}

// HasColumn reports whether a column with the given name (case-insensitive)
// exists.
func (d *Dataset) HasColumn(name string) bool {
	_, ok := d.columnIndex(name)
	return ok
}

func (d *Dataset) columnIndex(name string) (int, bool) {
	ln := strings.ToLower(name)
	for i, c := range d.Columns {
		if strings.ToLower(c.Name) == ln {
			return i, true
		}
	}
	return 0, false
}

// AddColumn appends a column if one with the same name does not already exist.
// It returns true if the column was added.
func (d *Dataset) AddColumn(c Column) bool {
	if d.HasColumn(c.Name) {
		return false
	}
	d.Columns = append(d.Columns, c)
	return true
}

// ColumnNames returns the column display names in order.
func (d *Dataset) ColumnNames() []string {
	names := make([]string, len(d.Columns))
	for i, c := range d.Columns {
		names[i] = c.Name
	}
	return names
}

// AppendRow adds a row to the dataset.
func (d *Dataset) AppendRow(r Row) { d.Rows = append(d.Rows, r) }

// NObs is the number of observations (rows).
func (d *Dataset) NObs() int { return len(d.Rows) }

// Get returns the value of a column in a row (case-insensitive). If the column
// is absent it returns the appropriate missing value for the column's type, or
// numeric missing if the column is unknown.
func (d *Dataset) Get(r Row, name string) Value {
	v, ok := r[strings.ToLower(name)]
	if ok {
		return v
	}
	if idx, found := d.columnIndex(name); found && d.Columns[idx].Kind == Character {
		return MissingChar()
	}
	return MissingNum()
}

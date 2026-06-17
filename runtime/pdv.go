package runtime

import (
	"strings"

	"github.com/solifugus/ass/table"
)

// PDV is the Program Data Vector: the working set of variables for one DATA step
// iteration. It maps variable names to their current values and preserves the
// declaration order and type of each variable so the output dataset can be built
// with stable column order.
//
// Variable names are case-insensitive; they are stored lowercased for lookup and
// the original display name is preserved in the column order. Unlike a plain map,
// the PDV knows a variable's type (numeric vs character) once it has been
// declared, so an absent value reads back as the correctly typed missing value —
// matching SAS, where a variable's type is fixed the first time it appears.
type PDV struct {
	values   map[string]table.Value
	kinds    map[string]table.Kind
	order    []string            // display names, in first-seen order
	retained map[string]bool     // names exempt from per-iteration reset (lowercased)
	arrays   map[string][]string // array name (lowercased) -> element variable names
}

// NewPDV creates an empty PDV.
func NewPDV() *PDV {
	return &PDV{
		values:   make(map[string]table.Value),
		kinds:    make(map[string]table.Kind),
		retained: make(map[string]bool),
		arrays:   make(map[string][]string),
	}
}

// DefineArray registers an array's element variable names under its name.
func (p *PDV) DefineArray(name string, elements []string) {
	p.arrays[strings.ToLower(name)] = elements
}

// ArrayElement returns the variable name for the 1-based index of an array, and
// whether the array and index are valid.
func (p *PDV) ArrayElement(name string, index int) (string, bool) {
	elems, ok := p.arrays[strings.ToLower(name)]
	if !ok || index < 1 || index > len(elems) {
		return "", false
	}
	return elems[index-1], true
}

// Retain marks a variable as retained: ResetVars will not clear it between
// implicit-loop iterations.
func (p *PDV) Retain(name string) { p.retained[strings.ToLower(name)] = true }

// Declare registers a variable with a known type without setting a value. If the
// variable already exists its type is left unchanged (SAS fixes a variable's type
// at first appearance). The variable reads back as the typed missing value until
// it is set.
func (p *PDV) Declare(name string, kind table.Kind) {
	key := strings.ToLower(name)
	if _, ok := p.kinds[key]; ok {
		return
	}
	p.kinds[key] = kind
	p.order = append(p.order, name)
}

// Set assigns a value to a variable, declaring it (with the value's type) on
// first use.
func (p *PDV) Set(name string, v table.Value) {
	key := strings.ToLower(name)
	if _, ok := p.kinds[key]; !ok {
		p.kinds[key] = v.Kind
		p.order = append(p.order, name)
	}
	p.values[key] = v
}

// Get returns the current value of a variable. An undeclared variable reads as
// numeric missing; a declared-but-unset variable reads as its typed missing
// value.
func (p *PDV) Get(name string) table.Value {
	key := strings.ToLower(name)
	if v, ok := p.values[key]; ok {
		return v
	}
	if k, ok := p.kinds[key]; ok {
		if k == table.Character {
			return table.MissingChar()
		}
		return table.MissingNum()
	}
	return table.MissingNum()
}

// Has reports whether a variable has been declared.
func (p *PDV) Has(name string) bool {
	_, ok := p.kinds[strings.ToLower(name)]
	return ok
}

// Kind returns the declared type of a variable and whether it is declared.
func (p *PDV) Kind(name string) (table.Kind, bool) {
	k, ok := p.kinds[strings.ToLower(name)]
	return k, ok
}

// Names returns the declared variable display names in first-seen order.
func (p *PDV) Names() []string {
	out := make([]string, len(p.order))
	copy(out, p.order)
	return out
}

// ResetVars sets every declared variable back to its typed missing value. SAS
// clears the PDV at the top of each implicit-loop iteration (except retained and
// read-in variables, handled by the loop driver). Declarations and order are
// preserved.
func (p *PDV) ResetVars() {
	for key := range p.values {
		if !p.retained[key] {
			delete(p.values, key)
		}
	}
}

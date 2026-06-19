package sas7bdat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/solifugus/ass/table"
)

// DirBackend is a base/directory LIBNAME engine: a libref bound to a filesystem
// directory whose members are .sas7bdat files. It implements table.Backend, so a
// `libname mylib "/path";` makes `set mylib.member;` and `proc print
// data=mylib.member;` read the file mylib/member.sas7bdat. Read-only.
type DirBackend struct {
	dir string
}

// OpenDir binds a directory as a base library. The path must be an existing
// directory.
func OpenDir(dir string) (*DirBackend, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("libname directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("libname path %q is not a directory", dir)
	}
	return &DirBackend{dir: dir}, nil
}

// Load reads member.sas7bdat from the library directory. SAS dataset files are
// conventionally lowercase; the lookup tries the member as given, lowercased,
// and uppercased. ok is false when no matching file exists.
func (b *DirBackend) Load(member string) (*table.Dataset, bool, error) {
	for _, name := range []string{member, strings.ToLower(member), strings.ToUpper(member)} {
		path := filepath.Join(b.dir, name+".sas7bdat")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		ds, err := Read(path)
		if err != nil {
			return nil, false, err
		}
		ds.Name = member
		return ds, true, nil
	}
	return nil, false, nil
}

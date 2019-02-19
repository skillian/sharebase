package main

import (
	"os"
	"strings"

	"github.com/skillian/errors"
)

const pathSep = string(os.PathSeparator)

// Path for our needs is just a slice of strings where the first string is the
// Library name, the last string is the Object's name and all the middle
// strings are parent folders.
type Path []string

// LocalPath creates a Path from the given local path.
func LocalPath(value string) Path {
	parts := strings.Split(value, string(os.PathSeparator))
	// remove empty pieces:
	d := 0
	for i := 0; i < len(parts); i++ {
		if parts[i] == "" {
			d--
			continue
		}
		parts[i+d] = parts[i]
	}
	return parts[:len(parts)+d]
}

// PathOf gets the path to the given object.  It does this by getting the
// ParentsOf the object and reversing them, ommitting the Root.
func PathOf(o Object) Path {
	parents := ParentsOf(o)
	if len(parents) == 0 {
		if r, ok := o.(*Root); !ok {
			panic(errors.NewUnexpectedType(r, o))
		}
		return Path([]string{"/"})
	}
	path := make(Path, len(parents))
	for i, parent := range parents[:len(parents)-1] {
		path[len(path)-2-i] = parent.Name()
	}
	path[len(path)-1] = o.Name()
	return path
}

// ShareBasePath creates a Path from the given ShareBase path by splitting
// the path by the ShareBase path separator.  If the ShareBase URI scheme
// is at the beginning of the string, it is removed.  If the library name is
// "my," it is expanded to the full true library name: "My Library."
func ShareBasePath(value string) Path {
	if strings.HasPrefix(value, shareBaseURIScheme) {
		value = value[len(shareBaseURIScheme):]
	}
	parts := LocalPath(value)
	if len(parts) > 0 {
		if parts[0] == "my" {
			parts[0] = "My Library"
		}
	}
	return Path(parts)
}

// Dir gets the parent directory of the given path.
func (p Path) Dir() Path {
	length := len(p)
	if length == 0 {
		return nil
	}
	return p[0 : length-1]
}

// Cmp compares two paths by comparing the strings within them, from first
// to last.  If the paths are the same length and their strings compare equal,
// 0 is returned.  If the prefixes of both paths match, Cmp returns the result
// of comparing the paths lengths.
func (p Path) Cmp(p2 Path) int {
	min := len(p)
	if len(p2) < min {
		min = len(p2)
	}
	for i, part := range p[:min] {
		cmp := strings.Compare(part, p2[i])
		if cmp != 0 {
			return cmp
		}
	}
	if len(p) == len(p2) {
		return 0
	}
	if min == len(p) {
		return -1
	}
	return 1
}

// LibraryName gets the name of the library that holds the object with this
// path.Folder
func (p Path) LibraryName() string {
	return p[0]
}

// Name gets the object's own name within the path.
func (p Path) Name() string {
	return p[len(p)-1]
}

// StartsWith determines if the given path starts with the given prefix.
// StartsWith returns true if path and prefix are the same full path.
func (p Path) StartsWith(prefix Path) bool {
	if len(prefix) > len(p) {
		return false
	}
	for i, part := range prefix {
		if p[i] != part {
			return false
		}
	}
	return true
}

// String returns the string representation of the Path.
func (p Path) String() string {
	return strings.Join(p, pathSep)
}

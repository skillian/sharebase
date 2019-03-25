package main

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// PathSeparator is the separator used to describe paths in ShareBase from
// this command.
//
// I know it's confusing because ShareBase's API actually uses backslashes like
// Windows, but this command's implementation uses the forward slash because
// ShareBase paths also use a made up URI format and URIs' paths use forward
// slashes.
const PathSeparator = "/"

// Path is the interface implemented by all filesystem paths, either local
// or in ShareBase.
type Path interface {
	// Dir gets the parent directory of this path.
	Dir() Path

	// Elem gets a path element by its index.
	Elem(index int) string

	// Len gets the number of elements in the path.
	Len() int

	// String produces a string representation of the path.
	String() string
}

// PathOf gets the ShareBasePath of the given ShareBase Object.  It builds this
// path by traversing the object's parents.
func PathOf(o Object) ShareBasePath {
	parents := ParentsOf(o)
	if len(parents) == 0 {
		return ShareBasePath{}
	}
	parts := make([]string, len(parents))
	for i, parent := range parents[:len(parents)-1] {
		parts[len(parts)-2-i] = parent.Name()
	}
	parts[len(parts)-1] = o.Name()
	return ShareBasePath(parts)
}

// Basename gets a path's base name (i.e. the last element of the path).
func Basename(p Path) string {
	length := p.Len()
	if length == 0 {
		return ""
	}
	return p.Elem(length - 1)
}

// LocalPath describes a path to somewhere on the local device's filesystem.
type LocalPath []string

// LocalPathFromString creates a LocalPath from the given path string.
func LocalPathFromString(v string) LocalPath {
	v = filepath.Clean(v)
	return LocalPath(strings.Split(v, string(os.PathSeparator)))
}

// LocalPathFromPaths creates a single LocalPath from the given path parts
// by joining them together.
func LocalPathFromPaths(ps ...Path) LocalPath {
	return LocalPath(joinPathElems(ps))
}

// Copy creates a copy of the LocalPath.
func (p LocalPath) Copy() LocalPath {
	p2 := make(LocalPath, len(p))
	copy(p2, p)
	return p2
}

// Dir implements the Path interface.
func (p LocalPath) Dir() Path { return p[:len(p)-1] }

// Elem implements the Path interface.
func (p LocalPath) Elem(index int) string { return p[index] }

// Len implements the Path interface.
func (p LocalPath) Len() int { return len(p) }

// String implements the Path interface.
func (p LocalPath) String() string { return filepath.Join([]string(p)...) }

// ShareBasePath describes a path to a ShareBase location.
type ShareBasePath []string

// ShareBasePathFromPaths creates a single ShareBasePath from the given path
// parts by joining them together.
func ShareBasePathFromPaths(ps ...Path) ShareBasePath {
	return ShareBasePath(joinPathElems(ps))
}

// ShareBasePathFromString creates a ShareBasePath from the given string path.
//
// Note that unlike ShareBase's API, ShareBase paths in this program actually
// use forward slashes.
func ShareBasePathFromString(v string) ShareBasePath {
	if strings.HasPrefix(v, shareBaseURIScheme) {
		v = v[len(shareBaseURIScheme):]
	}
	v = path.Clean(v)
	elems := strings.Split(v, PathSeparator)
	if len(elems) > 0 {
		if elems[0] == "my" {
			elems[0] = "My Library"
		}
	}
	for i, elem := range elems {
		fixed := strings.Join(
			allowedShareBaseRegexp.FindAllString(
				elem, -1),
			"")
		if elem != fixed {
			logger.Warn(
				"invalid ShareBase path element: %q " +
					"changed to: %q",
				elem, fixed)
			elems[i] = fixed
		}
	}
	return ShareBasePath(elems)
}

var allowedShareBaseRegexp = regexp.MustCompile(
	"[0-9A-Za-z_\\.\\-\\+ ]+")

// Copy creates a copy of the ShareBasePath.
func (p ShareBasePath) Copy() ShareBasePath {
	p2 := make(ShareBasePath, len(p))
	copy(p2, p)
	return p2
}

// Dir implements the Path interface.
func (p ShareBasePath) Dir() Path { return p[:len(p)-1] }

// Elem implements the Path interface.
func (p ShareBasePath) Elem(index int) string { return p[index] }

// Len implements the Path interface.
func (p ShareBasePath) Len() int { return len(p) }

// String implements the Path interface.
func (p ShareBasePath) String() string {
	return shareBaseURIScheme + path.Join([]string(p)...)
}

func joinPathElems(ps []Path) []string {
	if len(ps) == 0 {
		return nil
	}
	length := 0
	for _, p := range ps {
		length += p.Len()
	}
	parts := make([]string, length)
	i := 0
	for _, p := range ps {
		length = p.Len()
		for j := 0; j < length; j++ {
			parts[i] = p.Elem(j)
			i++
		}
	}
	return parts
}

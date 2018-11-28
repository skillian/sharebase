package web

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/skillian/errors"
	"github.com/skillian/logging"
)

const (
	// PathSep is the character used to separate components of the ShareBase
	// path.
	PathSep = "\\"
)

var (
	logger = logging.GetLogger("github.com/skillian/sharebase")
)

// Lener is implemented by types that have a Len method returning their
// length in bytes.  It is used by the Folder.NewDocument function that picks
// the recommended upload type based on the file size.
type Lener interface {
	// Len gets the length of the type's storage in bytes.
	Len() int
}

// mergeURL merges non-empty source fields into the given target.
func mergeURL(target, source *url.URL) {
	if source == nil || target == nil {
		return
	}
	if source.Scheme != "" {
		target.Scheme = source.Scheme
	}
	if source.Opaque != "" {
		target.Opaque = source.Opaque
	}
	if source.User != nil {
		target.User = source.User
	}
	if source.Host != "" {
		target.Host = source.Host
	}
	if source.Path != "" {
		target.Path = source.Path
	}
	if source.RawPath != "" {
		target.RawPath = source.RawPath
	}
	target.ForceQuery = source.ForceQuery
	if source.RawQuery != "" {
		target.RawQuery = source.RawQuery
	}
	if source.Fragment != "" {
		target.Fragment = source.Fragment
	}
}

func parseURL(urlString string) (*url.URL, error) {
	urlURL, err := url.Parse(urlString)
	if err != nil {
		return nil, errors.ErrorfWithCause(
			err,
			"failed to parse %q as URL: %v",
			urlString, err)
	}
	return urlURL, nil
}

// stringNotEmpty is the Go equivalent of an ArgumentNullException on a string
// argument in the .NET Common Language Runtime.
func stringNotEmpty(v, name string) (string, error) {
	if len(v) == 0 {
		return v, errors.Errorf("%q cannot be empty", name)
	}
	return v, nil
}

// Concat appends strings together without a separator
func Concat(values ...string) string {
	return strings.Join(values, "")
}

// Kind is used when wrapping any ShareBase object to indicate what kind it is.
type Kind string

const (
	// LibraryKind is a ShareBase library
	LibraryKind Kind = "Library"

	// FolderKind is a ShareBase Folder.
	FolderKind Kind = "Folder"

	// DocumentKind is a ShareBaseDocument
	DocumentKind Kind = "Document"
)

// String implements the Stringer interface.
func (k Kind) String() string {
	return string(k)
}

// NotFound is an error returned when a ShareBase object cannot be found.
type NotFound struct {
	Kind
	ID   int
	Name string
}

// Error implements the error interface.
func (err NotFound) Error() string {
	key := err.Name
	if key == "" {
		key = strconv.Itoa(err.ID)
	}
	return fmt.Sprintf("%v %v not found", err.Kind, key)
}

type statusError struct {
	code int
	msg  string
}

func (err statusError) Error() string {
	return fmt.Sprintf(
		"status %d: %v", err.code, err.msg)
}

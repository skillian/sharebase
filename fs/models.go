package fs

import (
	"github.com/skillian/sharebase/web"
)

// Entry describes any file system entry.  Entries are not immutable and and
// are updated after they are refreshed instead of requiring consumers of the
// API to check if they're still valid.
type Entry interface {
	// ID gets the ID of an entry.
	ID() EntryID

	// Name gets the name of the entry within its parent directory
	Name() EntryName

	// Directory is the container that holds this entry.
	Directory() Directory
}

// File describes a ShareBase document in a folder.
type File struct {
	entry
	*web.Document
}

// Directory is a file system directory
type Directory interface {
	// Entry because a Directory is itself an entry.
	Entry

	// Entries gets a slice of the entries within the directory.  This
	// slice may be borrowed so do not modify it.
	Entries() []Entry

	// EntryByName retrieves a single entry from the directory by its
	// local name.
	EntryByName(name string) (Entry, bool)
}

// Folder is a Directory implementation backed by a ShareBase folder.
type Folder struct {
	entry
	entries
	lib *Library
	*web.Folder
}

func (f *Folder) library() *Library { return f.lib }

// Library is a Directory implementation backed by a ShareBase Library.
type Library struct {
	entry
	entries
	*web.Library

	// documents has an index of all documents in the library by their IDs
	documents map[int]*Document

	// folders has an index of all folders in the library by their IDs
	folders map[int]*Folder
}

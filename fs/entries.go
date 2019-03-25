package fs

// EntryID is the ID of a file system entry.
type EntryID interface {
	// ID gets the actual integer value of the EntryID.
	ID() int

	// Kind distinguishes between a document and a folder's IDs.
	Kind() EntryKind
}

// EntryKind determines the type of entry.
type EntryKind byte

const (
	// DocumentKind signifies that an EntryID is a Document ID.
	DocumentKind EntryKind = iota

	// FolderKind signifies that an EntryID is a Folder ID.
	FolderKind
)

// DocumentID is the ID of a Document.
type DocumentID int

// ID implements the EntryID interface.
func (d DocumentID) ID() int { return int(d) }

// Kind implements the EntryID interface.
func (d DocumentID) Kind() EntryKind { return DocumentKind }

// FolderID is the ID of a Folder.
type FolderID int

// ID implements the EntryID interface.
func (f FolderID) ID() int { return int(f) }

// Kind implements the EntryID interface.
func (f FolderID) Kind() EntryKind { return FolderKind }

// EntryName is the name of a file system entry.
type EntryName interface {
	// Name gets the actualstring value of the EntryName.
	Name() string

	// Kind distinguishes between a document and a folder's IDs.
	Kind() EntryKind
}

// DocumentName holds the name of a document.
type DocumentName string

// Name implements the EntryName interface.
func (n DocumentName) Name() string { return string(n) }

// Kind implements the EntryName interface.
func (n DocumentName) Kind() EntryKind { return DocumentKind }

// FolderName holds the name of a folder.
type FolderName string

// Name implements the EntryName interface.
func (n FolderName) Name() string { return string(n) }

// Kind implements the EntryName interface.
func (n FolderName) Kind() EntryKind { return FolderKind }

type entry struct {
	id   EntryID
	name EntryName
	dir  Directory
}

func (e entry) ID() EntryID          { return e.id }
func (e entry) Name() EntryName      { return e.name }
func (e entry) Directory() Directory { return e.dir }

// entries contains a collection of entries indexed by their names and IDs.
type entries struct {
	// items is all of the entries in the collection.
	items []Entry

	// names holds a mapping of the entry names to their indexes in the
	// items slice
	names map[EntryName]int

	// ids holds a mapping of the entry IDs to their indexes in the
	// items slice
	ids map[EntryID]int
}

func (e *entries) Entries() []Entry { return e.items }

func (e *entries) EntryByID(id EntryID) (Entry, bool) {
	i, ok := e.ids[id]
	if ok {
		return e.items[i], true
	}
	return nil, false
}

func (e *entries) EntryByName(name EntryName) (Entry, bool) {
	i, ok := e.names[name]
	if !ok {
		return e.items[i], true
	}
	return nil, false
}

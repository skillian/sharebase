package main

import (
	"io"

	"github.com/skillian/errors"
	"github.com/skillian/sharebase/web"
)

// Root is a ShareBase filesystem root that keeps track of the top-level
// libraries inside.
type Root struct {
	// objects contains the collection of libraries known by a client.
	objects

	// idCache is a lookup from an object's ID to a library, folder, or
	// document with that ID (it's possible for instances of any of those
	// types to happen to have the same ID).
	idCache map[int]libFldDoc

	// missing is a lookup of objects that were not found while updating
	// the Root, such as when a document was in a folder but after calling
	// update, the document isn't in that folder any more.  Instead of
	// letting it be reclaimed by GC, it's put into the missing map so
	// the existing object can be updated
	missing map[int]libFldDoc
}

// NewRoot creates a new ShareBase root.
func NewRoot() *Root {
	r := new(Root)
	r.objects.init(8)
	r.idCache = make(map[int]libFldDoc)
	r.missing = make(map[int]libFldDoc)
	return r
}

// ParentByPath retrieves a parent library or folder for the given full path as
// well as the name of the last element within the path.  If origin is not nil,
// path is a relative path to origin, otherwise path is an absolute path
// including the library name.
//
// For example:
//
//     p, base, err := r.ParentByPath(c, nil, "sb:my/Documents/test.txt")
//     fmt.Printf("%q, %q, %v\n", PathOf(p), base, err)
//
// Output:
//
//     "My Library/Documents", "test.txt", nil
//
// The last child isn't retrieved in case it is the target of an upload
// operation and doesn't exist yet.
func (r *Root) ParentByPath(c *web.Client, origin Parent, path Path) (p Parent, base string, err error) {
	dir := path.Dir()
	logger.Debug1("dir: %v", dir)
	o, err := r.ObjectByPath(c, origin, path.Dir())
	if err != nil {
		return nil, "", errors.Errorf(
			"failed to get parent directory %q of path %q: %v",
			dir, path, err)
	}
	p, ok := o.(Parent)
	if !ok {
		return nil, "", errors.Errorf(
			"object %v exists but is not a parent", path)
	}
	return p, Basename(path), nil
}

// GetOrCreateFolder attempts to get an existing folder with the given path
// but creates it if necessary.
func (r *Root) GetOrCreateFolder(c *web.Client, origin Parent, path Path) (f *Folder, err error) {
	logger.Debug2("origin: %#v, path: %#v", PathOf(origin), path)
	o, err := r.ObjectByPath(c, origin, path)
	if err == nil {
		f, ok := o.(*Folder)
		if ok {
			return f, nil
		}
		return nil, errors.NewUnexpectedType(f, o)
	} else if _, ok := err.(ChildNotFound); !ok {
		return nil, errors.ErrorfWithCause(
			err,
			"error while checking for existing folder %v",
			Basename(path))
	}
	fullPath := ShareBasePathFromPaths(PathOf(origin), path)
	lib, err := r.LibraryByName(fullPath.Elem(0))
	if err != nil {
		return nil, errors.ErrorfWithCause(
			err,
			"failed to get library %q", fullPath.Elem(0))
	}
	if _, err = lib.Library.NewFolder(c, fullPath[1:]...); err != nil {
		return nil, errors.ErrorfWithCause(
			err,
			"failed to create folder %v: %v", fullPath, err)
	}
	// now we need to refresh the tree up from the origin to build up the
	// full path.
	o, err = r.ObjectByPath(c, origin, path)
	if err != nil {
		return nil, errors.ErrorfWithCause(
			err,
			"failed to retrieve folder %v we just created",
			fullPath)
	}
	f, ok := o.(*Folder)
	if !ok {
		return nil, errors.Errorf(
			"created a folder but it's not a folder anymore???")
	}
	return f, nil
}

// ObjectByPath retrieves an Object from the ShareBase API by its path,
// relative to the origin.  If origin is nil, path must be a full path,
// including the library name.
func (r *Root) ObjectByPath(c *web.Client, origin Parent, path Path) (Object, error) {
	if origin == nil {
		origin = r
	}
	o := Object(origin)
	//for _, part := range path {
	for i := 0; i < path.Len(); i++ {
		part := path.Elem(i)
		logger.Debug2("Getting child %q from parent %v...", part, PathOf(o))
		p, ok := o.(Parent)
		if !ok {
			return nil, errors.Errorf(
				"expected folder or library, not %T", o)
		}
		o, ok = p.ChildByName(part)
		if !ok {
			if err := p.update(r, c); err != nil {
				return nil, err
			}
			// Try again.
			if o, ok = p.ChildByName(part); !ok {
				// No handling if it fails after update.
				return nil, ChildNotFound{Name: part}
			}
		}
	}
	return o, nil
}

// ID is a "dummy" function just to implement the Object interface.
func (r *Root) ID() int { return 0 }

// LibraryByName retrieves a library by its given name.
func (r *Root) LibraryByName(name string) (*Library, error) {
	c, ok := r.objects.ChildByName(name)
	if !ok {
		return nil, ChildNotFound{Name: name}
	}
	lib, ok := c.(*Library)
	if !ok {
		return nil, errors.NewUnexpectedType(lib, c)
	}
	return lib, nil
}

// Name is a "dummy" function just to implement the Object interface.
func (r *Root) Name() string { return "" }

// Parent is a "dummy" function just to implement the Object interface.
func (r *Root) Parent() Parent { return nil }

func (r *Root) update(r2 *Root, c *web.Client) error {
	if r != r2 {
		panic("updating a Root from another root!?")
	}
	wls, err := c.Libraries()
	if err != nil {
		return err
	}
	libs := objects{}
	libs.init(len(wls))
	for _, wl := range wls {
		lfd := r.idCache[wl.LibraryID]
		if lfd.Library == nil {
			// try to reclaim it from the missing map.
			if lfd2, ok := r.missing[wl.LibraryID]; ok {
				if lfd2.Library != nil {
					lfd.Library = lfd2.Library
					lfd2.Library = nil
					lfd2.updateMap(r.missing, wl.LibraryID)
				}
			} else {
				lfd.Library = newLibrary(r, wl)
			}
		}
		lfd.Library.Library = wl
		libs.add(lfd.Library)
		r.idCache[wl.LibraryID] = lfd
	}
	// update missing map.
	for _, c := range r.objects.Children() {
		id := c.ID()
		if _, ok := libs.ids[id]; !ok {
			lfd := r.missing[id]
			lfd.Library = c.(*Library)
			r.missing[id] = lfd
		}
	}
	r.objects = libs
	return nil
}

// updateObjects should only be called by Library and Folder's update
// implementations.
func (r *Root) updateObjects(p Parent, obs *objects, wfs []web.Folder, wds []web.Document) error {
	logger.Debug2("%v current children: %#v", p.Name(), p.Children())
	objects := new(objects)
	objects.init(len(wfs) + len(wds))
	for _, wf := range wfs {
		lfd := r.idCache[wf.FolderID]
		if lfd.Folder == nil {
			if lfd2, ok := r.missing[wf.FolderID]; ok {
				if lfd2.Folder != nil {
					lfd.Folder = lfd2.Folder
					lfd2.Folder = nil
					lfd2.updateMap(r.missing, wf.FolderID)
				}
			}
			lfd.Folder = newFolder(p, wf)
		}
		oldEmbed := lfd.Folder.Folder.Embedded
		lfd.Folder.Folder = wf
		lfd.Folder.Folder.Embedded = oldEmbed
		objects.add(lfd.Folder)
		r.idCache[wf.FolderID] = lfd
	}
	for _, wd := range wds {
		lfd := r.idCache[wd.DocumentID]
		if lfd.Document == nil {
			if lfd2, ok := r.missing[wd.DocumentID]; ok {
				if lfd2.Document != nil {
					lfd.Document = lfd2.Document
					lfd2.Document = nil
					if lfd2.empty() {
						delete(r.missing, wd.DocumentID)
					} else {
						r.missing[wd.DocumentID] = lfd2
					}
				}
			} else {
				lfd.Document = &Document{Folder: p.(*Folder)}
			}
		}
		lfd.Document.Document = wd
		objects.add(lfd.Document)
		r.idCache[wd.DocumentID] = lfd
	}
	for _, c := range obs.Children() {
		id := c.ID()
		if _, ok := objects.ids[id]; !ok {
			lfd := r.missing[id]
			switch c := c.(type) {
			case *Document:
				lfd.Document = c
			case *Folder:
				lfd.Folder = c
			default:
				panic(errors.Errorf(
					"invalid object type: %T", c))
			}
			r.missing[id] = lfd
		}
	}
	*obs = *objects
	logger.Debug2("%v new children: %#v", p.Name(), p.Children())
	return nil
}

// Object is the basic interface implemented by all ShareBase objects
type Object interface {
	// ID gets the ID of the specific object.  IDs are not necessarily
	// unique across object types (e.g. there might be a Library with ID 1
	// and a Folder with ID 1).
	ID() int

	// Name gets the name of the object.  Names are unique directly under
	// a parent object but are not unique across the whole system.
	Name() string

	// Parent gets the Child object's parent object.
	Parent() Parent

	// update the current object with the latest state from the web client.
	// This call should not recurse into the children, but should update
	// the children if it can be done in a single call (e.g. with the embed
	// GET parameter on folders).  Implementations can assume the parent
	// was just updated before calling, so they don't have to deal with
	// their parents being out of sync.
	update(r *Root, c *web.Client) error
}

// ParentsOf climbs an object's Parent() chain until it gets to the root.
// The returned slice is in child to parent order, with the last Parent being
// the Root.
func ParentsOf(o Object) (parents []Parent) {
	for p := o.Parent(); p != nil; p = p.Parent() {
		parents = append(parents, p)
	}
	return
}

// Parent is a superset of the Object interface for any object that contains
// other objects.
type Parent interface {
	Object

	// ChildByName retrieves a child object by its Name.
	ChildByName(name string) (Object, bool)

	// Children returns all of a parent's children.  The returned slice
	// may be borrowed, so do not modify it.
	Children() []Object
}

// Traverse performs a breadth-first traversal of p's children without recursion
// and calls funcion f on all of them until f returns a non-nil error.  If
// f returns io.EOF, Walk breaks but nil is returned to the caller.
func Traverse(p Parent, f func(p Parent, c Object) error) error {
	type parentChild struct {
		Parent
		Child Object
	}
	appendChildren := func(pcs *[]parentChild, p Parent) {
		for _, c := range p.Children() {
			*pcs = append(*pcs, parentChild{
				Parent: p,
				Child:  c,
			})
		}
	}
	var pcs []parentChild
	appendChildren(&pcs, p)
	for len(pcs) > 0 {
		pc := pcs[0]
		pcs = pcs[1:]
		if err := f(pc.Parent, pc.Child); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if p, ok := pc.Child.(Parent); ok {
			appendChildren(&pcs, p)
		}
	}
	return nil
}

// FolderContainer is a parent of Folder objects.  Its FolderByName function
// probably calls a ChildByName function and then type-asserts the value to a
// Folder.
type FolderContainer interface {
	Parent
	FolderByName(name string) (*Folder, error)
}

// DocumentContainer is a parent of Document objects.  Similarly to
// the FolderContainer interface, the implementation likely just calls
// ChildByName and type-asserts the result.
type DocumentContainer interface {
	Parent
	DocumentByName(name string) (*Document, error)
}

// Library is a root ShareBase file system object.  Its children can only be
// folders.
type Library struct {
	*Root
	// Library gets the state loaded from the ShareBase API.
	web.Library
	folders
}

func newLibrary(r *Root, wl web.Library) *Library {
	lib := &Library{Root: r, Library: wl}
	lib.folders.init(0)
	return lib
}

// ChildByName implements the Parent interface.
func (l *Library) ChildByName(name string) (Object, bool) {
	return l.folders.ChildByName(name)
}

// Children implements the Parent interface.
func (l *Library) Children() []Object {
	return l.folders.Children()
}

// FolderByName implements the FolderContainer interface.
func (l *Library) FolderByName(name string) (*Folder, error) {
	c, ok := l.ChildByName(name)
	if !ok {
		return nil, ChildNotFound{Name: name}
	}
	f, ok := c.(*Folder)
	if !ok {
		return nil, errors.NewUnexpectedType(f, c)
	}
	return f, nil
}

// ID implements the Object interface.
func (l *Library) ID() int { return l.Library.LibraryID }

// Name implements the Object interface.
func (l *Library) Name() string { return l.Library.LibraryName }

// Parent implements the Object interface.
func (l *Library) Parent() Parent { return l.Root }

// update updates the nested folders with a single call.  It doesn't actually
// update its own state because it assumes it was just updated with a previous
// call to (*Root).update.
func (l *Library) update(r *Root, c *web.Client) error {
	wfs, err := l.Library.Folders(c)
	if err != nil {
		return err
	}
	return r.updateObjects(l, &l.folders.objects, wfs, nil)
}

// Folder is a ShareBase folder object.
type Folder struct {
	// DotDot is the direct parent of this object.  If this folder is
	// nested directly in a library, DotDot gets the library object.
	// If this is a subfolder, it gets the parent folder.
	DotDot Parent

	// Folder is the folder state loaded from the ShareBase API.
	web.Folder

	// objects contains the documents and folders in the folder.
	objects
}

func newFolder(p Parent, wf web.Folder) *Folder {
	f := &Folder{
		DotDot: p,
		Folder: wf,
	}
	f.objects.init(
		len(wf.Embedded.Documents) +
			len(wf.Embedded.Folders))
	return f
}

// ID implements the Object interface.
func (f *Folder) ID() int { return f.Folder.FolderID }

// Name implements the Object interface.
func (f *Folder) Name() string { return f.Folder.FolderName }

// Parent implements the Child interface.
func (f *Folder) Parent() Parent { return f.DotDot }

// ChildByName implements the Parent interface.
func (f *Folder) ChildByName(name string) (Object, bool) {
	return f.objects.ChildByName(name)
}

// Children gets all of a folder's child objects.
func (f *Folder) Children() []Object {
	return f.objects.Children()
}

// DocumentByName implements the DocumentContainer interface.
func (f *Folder) DocumentByName(name string) (*Document, error) {
	c, ok := f.ChildByName(name)
	if !ok {
		return nil, ChildNotFound{Name: name}
	}
	d, ok := c.(*Document)
	if !ok {
		return nil, errors.NewUnexpectedType(d, c)
	}
	return d, nil
}

// FolderByName implements the FolderContainer interface.
func (f *Folder) FolderByName(name string) (*Folder, error) {
	c, ok := f.ChildByName(name)
	if !ok {
		return nil, ChildNotFound{Name: name}
	}
	f2, ok := c.(*Folder)
	if !ok {
		return nil, errors.NewUnexpectedType(f2, c)
	}
	return f2, nil
}

func (f *Folder) update(r *Root, c *web.Client) error {
	wf, err := f.getWebUpdaterFunc()(c, f.Folder.FolderID)
	if err != nil {
		return err
	}
	return r.updateObjects(
		f, &f.objects, wf.Embedded.Folders, wf.Embedded.Documents)
}

func (f *Folder) getWebUpdaterFunc() func(c *web.Client, id int) (web.Folder, error) {
	switch p := f.Parent().(type) {
	case *Library:
		return p.Library.Folder
	case *Folder:
		return p.Folder.Folder
	default:
		panic(errors.Errorf(
			"invalid parent type: %T", p))
	}
}

// Document is a ShareBase document.
type Document struct {
	// Folder is the ShareBase folder that holds this Document.
	*Folder

	// Document holds the ShareBase API state of this document.
	web.Document
}

// ID implements the Object interface.
func (d *Document) ID() int { return d.Document.DocumentID }

// Name implements the Object interface.
func (d *Document) Name() string { return d.Document.DocumentName }

// Parent implements the Child interface.
func (d *Document) Parent() Parent { return d.Folder }

// update doesn't actually ever do anything because a document must be updated
// from its parent Folder's update method.
func (d *Document) update(r *Root, c *web.Client) error {
	return nil
}

type documents struct {
	objects
}

func (ds *documents) DocumentByName(name string) (*Document, error) {
	c, ok := ds.objects.ChildByName(name)
	if !ok {
		return nil, ChildNotFound{Name: name}
	}
	d, ok := c.(*Document)
	if !ok {
		return nil, errors.Errorf(
			"child object %v (type: %T) is not a %T",
			c, c, d)
	}
	return d, nil
}

type folders struct {
	objects
}

func (fs *folders) FolderByName(name string) (*Folder, error) {
	c, ok := fs.objects.ChildByName(name)
	if !ok {
		return nil, ChildNotFound{Name: name}
	}
	f, ok := c.(*Folder)
	if !ok {
		return nil, errors.Errorf(
			"child object %v (type: %T) is not a %T",
			c, c, f)
	}
	return f, nil
}

type objects struct {
	children []Object
	ids      map[int]int
	names    map[string]int
}

func (o *objects) ChildByID(id int) (Object, bool) {
	i, ok := o.ids[id]
	if !ok {
		return nil, false
	}
	return o.children[i], true
}

func (o *objects) ChildByName(name string) (Object, bool) {
	i, ok := o.names[name]
	if !ok {
		return nil, false
	}
	return o.children[i], true
}

func (o *objects) Children() []Object {
	return o.children
}

func (o *objects) init(capacity int) {
	if capacity < 0 {
		o.children = nil
		o.ids = make(map[int]int)
		o.names = make(map[string]int)
		return
	}
	o.children = make([]Object, 0, capacity)
	o.ids = make(map[int]int, capacity)
	o.names = make(map[string]int, capacity)
}

func (o *objects) add(c Object) (index int, added bool) {
	var ok bool
	id := c.ID()
	if _, ok = o.ids[id]; ok {
		return 0, false
	}
	name := c.Name()
	if _, ok = o.names[name]; ok {
		return 0, false
	}
	ix := len(o.children)
	o.children = append(o.children, c)
	o.ids[id] = ix
	o.names[name] = ix
	return ix, true
}

func (o *objects) del(c Object) (deleted bool) {
	id := c.ID()
	x0, ok := o.ids[id]
	if !ok {
		return false
	}
	name := c.Name()
	x1, ok := o.names[name]
	if !ok {
		return false
	}
	if x0 != x1 {
		return false
	}
	for i, c := range o.children[x0+1:] {
		o.children[x0+i] = c
		o.ids[c.ID()] = x0 + i
		o.names[c.Name()] = x0 + i
	}
	delete(o.ids, o.children[len(o.children)-1].ID())
	delete(o.names, o.children[len(o.children)-1].Name())
	return true
}

// libFldDoc is used by the Root type for tracking all of the objects by their
// IDs.
type libFldDoc struct {
	*Library
	*Folder
	*Document
}

func (lfd libFldDoc) empty() bool {
	return lfd.Library == nil && lfd.Folder == nil && lfd.Document == nil
}

func (lfd libFldDoc) merge(lfd2 libFldDoc) libFldDoc {
	if lfd2.Library != nil {
		lfd.Library = lfd2.Library
	}
	if lfd2.Folder != nil {
		lfd.Folder = lfd2.Folder
	}
	if lfd2.Document != nil {
		lfd.Document = lfd2.Document
	}
	return lfd
}

func (lfd libFldDoc) updateMap(m map[int]libFldDoc, id int) {
	if lfd.empty() {
		delete(m, id)
	} else {
		m[id] = lfd
	}
}

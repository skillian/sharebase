package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skillian/errors"
	"github.com/skillian/mparthelp"
)

// Size defines a file size in bytes
type Size int64

const (
	// B is the standard size unit of Bytes.
	B Size = 1 << (iota * 10)

	// K is kibibytes.
	K

	// M is mebibytes.
	M

	// G is gibibytes.
	G
)

const (
	// SmallFileCutoff is the file size limit under which ShareBase's
	// recommended small file upload method is used and above which ShareBase's
	// large file upload method is used.
	SmallFileCutoff Size = 5 * M

	// PatchSize is the size of patches made to a large file upload.
	// ShareBase's recommendation is 512K and not to exceed 2M, so I'm going
	// to go with the power of 2 between them.
	PatchSize Size = 512 * K
)

// LibraryLinks holds the URLs that a Library's Links attribute has.
type LibraryLinks struct {
	// Self is the library link.
	Self string

	// Folders is the URL that gets the folders within the library.
	Folders string
}

// Library is a library in ShareBase.
type Library struct {
	// LibraryID is the library's unique ID in ShareBase.
	LibraryID int `json:"LibraryId"`

	// LibraryName is the library's name.
	LibraryName string

	// IsPrivate is true for personal libraries and false otherwise.
	IsPrivate bool

	// Links holds the library's links to other resources.
	Links LibraryLinks
}

// Folders gets the collection of Folders under this Library.
func (lib *Library) Folders(c *Client) (folders []Folder, err error) {
	err = c.requestJSON(http.MethodGet, lib.Links.Folders, nil, &folders)
	return folders, err
}

// Folder gets a folder by ID from the given library.
func (lib *Library) Folder(c *Client, id int) (folder Folder, err error) {
	uriString := lib.Links.Folders + fmt.Sprintf("/%d?embed=d,f", id)
	err = c.requestJSON(http.MethodGet, uriString, nil, &folder)
	if _, ok := err.(NotFound); ok {
		return Folder{}, NotFound{Kind: FolderKind, ID: id, Name: ""}
	}
	return folder, err
}

// FolderByName gets a folder within the library by its name.
func (lib *Library) FolderByName(c *Client, name string) (Folder, error) {
	folders, err := lib.Folders(c)
	if err != nil {
		return Folder{}, err
	}
	for _, f := range folders {
		if f.FolderName == name {
			return f, nil
		}
	}
	return Folder{}, NotFound{Kind: FolderKind, ID: 0, Name: name}
}

// NewFolderRequest is used by the NewFolder function to create a new folder.
type NewFolderRequest struct {
	// FolderPath holds the full path to the folder with the path components
	// separated by backslashes.
	FolderPath string
}

// joinFolderPath joins path parts with backslashes.
func joinFolderPath(parts ...string) string {
	return strings.Join(parts, "\\")
}

// NewFolder creates a new folder within the library with the specified path.
func (lib *Library) NewFolder(c *Client, path ...string) (folder Folder, err error) {
	err = c.requestJSON(http.MethodPost, lib.Links.Folders, NewFolderRequest{
		FolderPath: joinFolderPath(path...),
	}, &folder)
	return folder, err
}

// Folder represents a folder in the ShareBase API.
type Folder struct {
	// FolderID is the unique ID of the folder in ShareBase.
	FolderID int `json:"FolderId"`

	// FolderName holds the name of the folder within its parent (i.e. not the
	// full path).
	FolderName string

	// LibraryID holds the ID of the library in which this folder resides.
	LibraryID int `json:"LibraryId"`

	// Links holds the folder's links to other objects.
	Links FolderLinks

	// Embedded holds nested objects that might be embedded in the folder.
	Embedded FolderEmbedded
}

// FolderLinks are the links specific to a folder.
type FolderLinks struct {
	// Self is the link to the same folder.
	Self string

	// Folders holds a link to this folder's subfolders.
	Folders string

	// Documents holds a link to this folder's nested documents.
	Documents string

	// Shares holds a link to get this folder's links.
	Shares string
}

// FolderEmbedded holds an embedded.
type FolderEmbedded struct {
	// Documents holds a slice of documents within the folder.
	Documents []Document

	// Folders holds a slice of folders within this folder.
	Folders []Folder
}

// Document attempts to retrieve a document from the folder by its ID.
func (f *Folder) Document(c *Client, id int) (Document, error) {
	docs, err := f.Documents(c)
	if err != nil {
		return Document{}, err
	}
	for _, d := range docs {
		if d.DocumentID == id {
			return d, nil
		}
	}
	return Document{}, NotFound{Kind: DocumentKind, ID: id, Name: ""}
}

// DocumentByName attempts to retrieve a document from the folder by its
// document name.
func (f *Folder) DocumentByName(c *Client, name string) (Document, error) {
	docs, err := f.Documents(c)
	if err != nil {
		return Document{}, err
	}
	for _, d := range docs {
		if d.DocumentName == name {
			return d, nil
		}
	}
	return Document{}, NotFound{Kind: DocumentKind, ID: 0, Name: name}
}

// Documents gets the documents embedded in this folder.
func (f *Folder) Documents(c *Client) (documents []Document, err error) {
	err = c.requestJSON(http.MethodGet, f.Links.Documents, nil, &documents)
	return documents, err
}

// Folders gets a slice of folders nested in this folder.
func (f *Folder) Folders(c *Client) (folders []Folder, err error) {
	err = c.requestJSON(http.MethodGet, f.Links.Folders, nil, &folders)
	return
}

// Folder gets a single folder within the current folder by its ID.
func (f *Folder) Folder(c *Client, id int) (folder Folder, err error) {
	uriString := f.Links.Folders + fmt.Sprintf("/%d?embed=d,f", id)
	err = c.requestJSON(http.MethodGet, uriString, nil, &folder)
	if _, ok := err.(NotFound); ok {
		return Folder{}, NotFound{Kind: FolderKind, ID: id, Name: ""}
	}
	return
}

// FolderByName gets a child folder from the current folder by its name.
func (f *Folder) FolderByName(c *Client, name string) (Folder, error) {
	folders, err := f.Folders(c)
	if err != nil {
		return Folder{}, err
	}
	for _, f := range folders {
		if f.FolderName == name {
			return f, nil
		}
	}
	return Folder{}, NotFound{Kind: FolderKind, ID: 0, Name: name}
}

// NewDocument creates a new ShareBase document in the given folder.
func (f *Folder) NewDocument(c *Client, name string, content io.Reader) error {
	if lengther, ok := content.(Lener); ok {
		if Size(lengther.Len()) < SmallFileCutoff {
			return f.newSmallDocument(c, name, content)
		}
	}
	return f.newLargeDocument(c, name, content)
}

// NewDocumentRequest is marshaled when creating a new document.
type NewDocumentRequest struct {
	// DocumentName is the name of the document to be created in a folder.
	DocumentName string
}

func (f *Folder) newSmallDocument(c *Client, name string, content io.Reader) error {
	body := bytes.Buffer{}
	formDataContentType, err := mparthelp.Parts{
		mparthelp.Part{
			Name:   "metadata",
			Source: mparthelp.JSON{Value: NewDocumentRequest{DocumentName: name}},
		},
		mparthelp.Part{
			Name:   "file",
			Source: mparthelp.File{Name: name, Reader: content, Closer: nil},
		},
	}.Into(&body)
	if err != nil {
		return err
	}
	err = c.request(
		http.MethodPost,
		f.Links.Documents,
		&body,
		nil,
		setContentType(formDataContentType))
	return err
}

// NewLargeDocumentResponse is a JSON response returned when creating a large
// document in ShareBase.
type NewLargeDocumentResponse struct {
	Links       NewLargeDocumentResponseLinks
	Identifier  uuid.UUID
	FileName    string
	CurrentSize uint64
	VolumeID    int `json:"VolumeId"`
}

// NewLargeDocumentResponseLinks is the embedded Links struct returned inside
// of the NewLargeDocumentResponse struct.
type NewLargeDocumentResponseLinks struct {
	Location string
}

// newLargeDocument uploads a large document.
func (f *Folder) newLargeDocument(c *Client, name string, content io.Reader) (err error) {
	// deleted everything 2018-11-25 14:16
	res, err := f.createNewLargeDocument(c, name)
	if err != nil {
		return errors.ErrorfWithCause(
			err, "failed to create new document request: %v", err)
	}
	dataBuffer := new(bytes.Buffer)
	dataBuffer.Grow(int(PatchSize))
	jsonBuffer := new(bytes.Buffer)
	cur := res
	// It'd be nice if this could be stack-allocated, but I think all values
	// passed as interfaces always escape to the heap:
	dataReader := &io.LimitedReader{R: content, N: 0}
	total := int64(0)
	for {
		dataReader.N = int64(PatchSize)
		// copying to a buffer instead of just passing the LimitedReader to
		// the request so that the content length can be known before reading
		// the body.  ShareBase requires the content length be specified or
		// else you end up with a 0 byte file in ShareBase.
		w, err := io.Copy(dataBuffer, dataReader)
		if err != nil {
			return errors.ErrorfWithCause(
				err, "failure buffering data for patch: %v", err)
		}
		if w == 0 {
			break
		}
		if err = c.request(http.MethodPatch, res.Links.Location, dataBuffer, jsonBuffer); err != nil {
			return errors.ErrorfWithCause(
				err, "failed to patch document %q: %v", name, err)
		}
		if err = json.Unmarshal(jsonBuffer.Bytes(), &cur); err != nil {
			return errors.ErrorfWithCause(
				err, "failed to unmarshal updated upload info: %v", err)
		}
		if cur.CurrentSize == 0 {
			return errors.Errorf(
				"Last patch of document %q uploaded nothing.", name)
		}
		jsonBuffer.Reset()
		total += w
	}
	var d Document
	err = c.requestJSON(http.MethodPost, f.Links.Documents, nil, &d, func(req *http.Request) error {
		b, err := json.Marshal(res)
		if err != nil {
			return err
		}
		req.Header["x-sharebase-fileref"] = []string{string(b)}
		return nil
	})
	logger.Debug1("Finished document %#v", d)
	return
}

// DocumentWriter creates a new document writer with the given document name
// under the current folder.  The DocumentWriter must be closed after writing!
func (f *Folder) DocumentWriter(c *Client, name string) (w *DocumentWriter, err error) {
	res, err := f.createNewLargeDocument(c, name)
	if err != nil {
		return nil, err
	}
	w = &DocumentWriter{
		Client: c,
		Folder: f,
		NewLargeDocumentResponse: res,
		dataBuffer:               bytes.Buffer{},
		jsonBuffer:               bytes.Buffer{},
	}
	w.dataBuffer.Grow(int(PatchSize))
	return
}

// DocumentWriter is returned by a folder's DocumentWriter function.  It
// implements the io.Writer interface and takes its written data and writes
// it to ShareBase.  A DocumentWriter must be closed after using it!
type DocumentWriter struct {
	*Client
	*Folder
	NewLargeDocumentResponse
	dataBuffer bytes.Buffer
	jsonBuffer bytes.Buffer
}

// Close finalizes the document upload.
func (w *DocumentWriter) Close() error {
	if w.dataBuffer.Len() > 0 {
		if err := w.patch(); err != nil {
			return err
		}
	}
	return w.Client.requestJSON(
		http.MethodPost,
		w.Folder.Links.Documents,
		nil,
		nil,
		func(req *http.Request) error {
			b, err := json.Marshal(w.NewLargeDocumentResponse)
			if err != nil {
				return err
			}
			req.Header["x-sharebase-fileref"] = []string{string(b)}
			return nil
		})
}

// Write implements the io.Writer interface.  It writes a chunk of a document
// to ShareBase with a PATCH.
func (w *DocumentWriter) Write(p []byte) (n int, err error) {
	if w.available() >= len(p) {
		return w.dataBuffer.Write(p)
	}
	for remaining := p; len(remaining) > 0; remaining = remaining[n:] {
		limit := w.available()
		if limit > len(remaining) {
			limit = len(remaining)
		}
		n, err = w.dataBuffer.Write(remaining[:limit])
		if err != nil {
			return
		}
		if err = w.patch(); err != nil {
			return
		}
	}
	return len(p), nil
}

// available returns the amount of space available in the patch buffer.
func (w *DocumentWriter) available() int {
	length := w.dataBuffer.Len()
	a := int(PatchSize) - length
	if a < 0 {
		panic(errors.Errorf(
			"%T data buffer larger than patch size (%d)", w, PatchSize))
	}
	return a
}

func (w *DocumentWriter) patch() (err error) {
	if err = w.Client.request(http.MethodPatch, w.NewLargeDocumentResponse.Links.Location, &w.dataBuffer, &w.jsonBuffer); err != nil {
		return errors.ErrorfWithCause(
			err, "failed to patch document %q: %v", w.NewLargeDocumentResponse.FileName, err)
	}
	if err = json.Unmarshal(w.jsonBuffer.Bytes(), &w.NewLargeDocumentResponse); err != nil {
		return errors.ErrorfWithCause(
			err, "failed to unmarshal updated upload info: %v", err)
	}
	if w.NewLargeDocumentResponse.CurrentSize == 0 {
		return errors.Errorf(
			"Last patch of document %q uploaded nothing.", w.NewLargeDocumentResponse.FileName)
	}
	w.jsonBuffer.Reset()
	return nil
}

// createNewLargeDocument posts a request for a temporary file in the folder
// with the given name.
func (f *Folder) createNewLargeDocument(c *Client, name string) (r NewLargeDocumentResponse, err error) {
	err = c.requestJSON(http.MethodPost, Concat(f.Links.Self, "/temp?filename=", name), nil, &r)
	return
}

// simpleReader is similar to a bytes.Reader but shouldn't require an
// allocation.
type simpleReader struct {
	b []byte
	i int
}

func (r *simpleReader) Read(b []byte) (n int, err error) {
	source := r.b[r.i:]
	if len(source) == 0 {
		return 0, io.EOF
	}
	limit := minInt(len(source), len(b))
	copy(b[:limit], source[:limit])
	r.i += limit
	return limit, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Document represents a single document in ShareBase.
type Document struct {
	// DocumentID holds the unique ID of the document in ShareBase.
	DocumentID int `json:"DocumentId"`

	// DocumentName holds the name of the document in its parent.
	DocumentName string

	// DateModified stores the date that the Document was last modified.
	DateModified time.Time

	// Links holds links that the document has to other ShareBase objects.
	Links DocumentLinks
}

// DocumentLinks holds a set of links to other objects.
type DocumentLinks struct {
	// Self holds a link back to the current object.
	Self string

	// Content holds a link to the document's content.
	Content string
}

// Content retrieves the document content.  It must be closed after it is
// retrieved.
func (d *Document) Content(c *Client) (DocumentContent, error) {
	head, body, err := c.requestBody(http.MethodGet, d.Links.Content, nil)
	if err != nil {
		return DocumentContent{}, err
	}
	lengths := head["Content-Length"]
	if len(lengths) == 0 {
		return DocumentContent{}, errors.Errorf(
			"%v content has no Content-Length", d)
	}
	bigLen := big.NewInt(0)
	if _, ok := bigLen.SetString(lengths[0], 10); !ok || !bigLen.IsInt64() {
		return DocumentContent{}, errors.Errorf(
			"failed to parse %v length %q to integer",
			d, lengths[0])
	}
	length := bigLen.Int64()
	var contentType string
	contentTypes := head["Content-Type"]
	if len(contentTypes) != 0 {
		contentType = contentTypes[0]
	}
	var contentDisposition string
	contentDispositions := head["Content-Disposition"]
	if len(contentDispositions) > 0 {
		contentDisposition = contentDispositions[0]
	}
	return DocumentContent{
		Document:           d,
		ReadCloser:         body,
		Length:             length,
		ContentType:        contentType,
		ContentDisposition: contentDisposition,
	}, nil
}

// String gets a string representation of the document.
func (d Document) String() string {
	return fmt.Sprintf("Document %q, (ID: %d)", d.DocumentName, d.DocumentID)
}

// DocumentContent unsurprisingly holds the content of the document.  It must be
// closed.
type DocumentContent struct {
	*Document
	io.ReadCloser
	Length             int64
	ContentType        string
	ContentDisposition string
}

// Close wraps the io.ReadCloser's Close function but adds more context to the
// message.
func (d DocumentContent) Close() error {
	err := d.ReadCloser.Close()
	if err != nil {
		return errors.ErrorfWithCause(
			err, "failed to close %v content: %v", d.Document, err)
	}
	return nil
}

// Len gets the document content's length as an int64 (A normal int isn't large
// enough on a 32-bit platform for file sizes >2GiB).
func (d DocumentContent) Len() int64 {
	return d.Length
}

package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/skillian/logging"

	"github.com/skillian/errors"
)

const (
	// PhoenixTokenPrefix is the string prefixed to ShareBase's token value
	// within the Authorization header of every request.
	PhoenixTokenPrefix = "PHOENIX-TOKEN "
)

var (
	authenticateURL = url.URL{Path: "api/authenticate"}
	documentsURL    = url.URL{Path: "api/documents"}
	foldersURL      = url.URL{Path: "api/folders"}
	librariesURL    = url.URL{Path: "api/libraries"}
)

// Client defines the client struct used to communicate with the ShareBase
// web API.
type Client struct {
	// httpClient is the http.Client used to actually make the REST
	// requests to the ShareBase API.
	httpClient http.Client

	// DataCenter holds the URL that should prefix all non-absolute
	// requests
	DataCenter url.URL

	// phoenixToken is a PHOENIX-TOKEN authorization header included in
	// all requests to the ShareBase API.
	phoenixToken string

	// numRequests keeps track of the total number of HTTP requests issued
	// to the ShareBase API.  It's accessible through the NumRequests
	// function.
	numRequests uint64
}

// NewClient creates a new client from the given dataCenter URL string and
// API token.
func NewClient(dataCenter, token string) (*Client, error) {
	if _, err := stringNotEmpty(dataCenter, "dataCenter"); err != nil {
		return nil, err
	}
	if _, err := stringNotEmpty(token, "token"); err != nil {
		return nil, err
	}
	dataCenterURL, err := parseURL(dataCenter)
	if err != nil {
		return nil, err
	}
	return &Client{
		httpClient:   http.Client{},
		DataCenter:   *dataCenterURL,
		phoenixToken: PhoenixTokenPrefix + token,
	}, nil
}

// AuthToken is the structure returned when authenticating with a username and
// password.  The Token field is passed to NewClient to make requests to the
// ShareBase API.
type AuthToken struct {
	// Token is the special authenticated value returned after a successful
	// authentication request.
	Token string

	// UserName repeats the username authenticated.
	UserName string

	// UserID gets the unique integer identifier of this user in the ShareBase
	// environment.
	UserID int `json:"UserId"`

	// ExpirationDate holds the time stamp when the authentication request
	// expires.
	ExpirationDate time.Time
}

// AuthTokenForUsernameAndPassword gets a ShareBase AuthToken for the given
// username and password combination.
func AuthTokenForUsernameAndPassword(dataCenter, username, password string) (authToken AuthToken, err error) {
	dataCenterURL, err := validateParamsForUserAndPass(dataCenter, username, password)
	if err != nil {
		return AuthToken{}, errors.ErrorfWithCause(err, "failed to validate parameters")
	}

	// request & response:
	dataCenterURLCopy := *dataCenterURL
	dataCenterURLCopy.Path = path.Join(dataCenterURLCopy.Path, authenticateURL.Path)
	req, err := http.NewRequest(http.MethodGet, dataCenterURLCopy.String(), nil)
	if err != nil {
		return AuthToken{}, errors.ErrorfWithCause(
			err,
			"failed to create authentication request: %v",
			err)
	}
	req.SetBasicAuth(username, password)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return AuthToken{}, errors.ErrorfWithCause(
			err,
			"error while authenticating user %q: %v",
			username, err)
	}

	// unmarshaling:
	buffer := new(bytes.Buffer)
	defer errors.WrapDeferred(&err, res.Body.Close)
	if _, err = io.Copy(buffer, res.Body); err != nil {
		return AuthToken{}, errors.ErrorfWithCause(err, "failed to copy response data")
	}
	bodyBytes := buffer.Bytes()
	if err = json.Unmarshal(bodyBytes, &authToken); err != nil {
		return AuthToken{}, errors.ErrorfWithCause(err, "failed to unmarshal %q as json: %#v", string(bodyBytes), err)
	}
	return
}

func validateParamsForUserAndPass(dataCenter, username, password string) (*url.URL, error) {
	_, err := stringNotEmpty(dataCenter, "dataCenter")
	if err != nil {
		return nil, err
	}
	_, err = stringNotEmpty(username, "username")
	if err != nil {
		return nil, err
	}
	_, err = stringNotEmpty(password, "password")
	if err != nil {
		return nil, err
	}
	return parseURL(dataCenter)
}

// Libraries gets all of the libraries accessible from the current Client.
func (c *Client) Libraries() (libraries []Library, err error) {
	libURL := c.DataCenter
	libURL.Path = path.Join(libURL.Path, librariesURL.Path)
	err = c.requestJSONURL(http.MethodGet, &libURL, nil, &libraries)
	return
}

// Library gets a library with the given integer ID.
func (c *Client) Library(id int) (library Library, err error) {
	libURL := c.DataCenter
	libURL.Path = path.Join(libURL.Path, librariesURL.Path, strconv.Itoa(id))
	err = c.requestJSONURL(http.MethodGet, &libURL, nil, &library)
	if err != nil {
		if _, ok := err.(NotFound); ok {
			return Library{}, NotFound{Kind: LibraryKind, ID: id, Name: ""}
		}
	}
	return
}

// LibraryByName attepts to retrieve a library by its name.
func (c *Client) LibraryByName(name string) (library Library, err error) {
	libs, err := c.Libraries()
	if err != nil {
		return Library{}, err
	}
	for _, lib := range libs {
		if lib.LibraryName == name {
			return lib, nil
		}
	}
	return Library{}, NotFound{Kind: LibraryKind, ID: 0, Name: name}
}

// NumRequests returns the total number of requests issued to the ShareBase API
// through this client.  It's useful for benchmarking to determine how many
// queries are consumed in case ShareBase ever switches to a per-request
// payment model.  This total includes both successful and failed requests.
func (c *Client) NumRequests() uint64 {
	return c.numRequests
}

// requestURL uses a URL when making a request.  Relative URLs are supported.
func (c *Client) requestURL(method string, uri *url.URL, source io.Reader, target io.Writer) error {
	if uri == nil {
		return errors.Errorf("uri cannot be nil")
	}
	uri2 := *uri
	c.setAbsURL(&uri2)
	return c.request(method, uri2.String(), source, target)
}

// requestOption is a function that potentially mutates the request before it is
// sent.
type requestOption func(req *http.Request) error

// setContentType is a request option that sets an HTTP request's content type.
func setContentType(v string) requestOption {
	return func(req *http.Request) error {
		req.Header.Set("Content-Type", v)
		return nil
	}
}

// requestBody creates a web-request and returns the response's body
// directly so it can be read from and closed without any copies
// in the middle.
func (c *Client) requestBody(method string, uri string, source io.Reader, options ...requestOption) (http.Header, io.ReadCloser, error) {
	if _, err := stringNotEmpty(method, "method"); err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest(method, uri, source)
	if err != nil {
		return nil, nil, errors.ErrorfWithCause(
			err,
			"failed to create request for %v: %v",
			uri, err)
	}
	req.Header.Set("Authorization", c.phoenixToken)
	req.Header["x-phoenix-app-id"] = []string{"ShareBase"}
	for _, o := range options {
		if err = o(req); err != nil {
			return nil, nil, errors.ErrorfWithCause(
				err,
				"error applying option: %v (type: %T): %v",
				o, o, err)
		}
	}
	if logger.Level() <= logging.VerboseLevel {
		buffer := bytes.Buffer{}
		if err = req.Write(&buffer); err != nil {
			return nil, nil, err
		}
		bufferBytes := buffer.Bytes()
		var bufferString string
		if len(bufferBytes) > 1024 {
			bufferString = string(bufferBytes[:1021]) + "..."
		} else {
			bufferString = string(bufferBytes)
		}
		logger.Log2(
			logging.VerboseLevel,
			"request (%d bytes total):\n\n%v",
			len(bufferBytes), bufferString)
	}
	res, err := c.httpClient.Do(req)
	c.numRequests++
	if err != nil {
		return nil, nil, errors.ErrorfWithCause(
			err,
			"failed to complete request for %v: %v",
			uri, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if res.StatusCode == 404 {
			// The caller must check if the result is NotFound and populate the
			// fields.
			return nil, nil, NotFound{}
		}
		return nil, nil, statusError{code: res.StatusCode, msg: res.Status}
	}
	return res.Header, res.Body, nil
}

// request a given relative or absolute URI with the given method.  The body
// of the request is populated from the source parameter and the response is
// copied to target.  Both source and/or target can be nil.
func (c *Client) request(method string, uri string, source io.Reader, target io.Writer, options ...requestOption) (err error) {
	_, body, err := c.requestBody(method, uri, source, options...)
	if err != nil {
		return err
	}
	defer errors.WrapDeferred(&err, body.Close)
	if target != nil {
		_, err = io.Copy(target, body)
		return
	}
	return nil
}

// absUrl modifies the given url.URL pointer to be absolute, using the
// Client's DataCenter configuration if uri is not already absolute.
func (c *Client) setAbsURL(uri *url.URL) {
	if uri.IsAbs() {
		return
	}
	uri.Scheme = c.DataCenter.Scheme
	uri.Host = c.DataCenter.Host
}

// requestJSON is similar to the request method but the source and target are
// marshaled to and unmarshaled from, respectively, JSON.
func (c *Client) requestJSON(method string, uri string, source, target interface{}, options ...requestOption) (err error) {
	var r io.Reader
	if source != nil {
		sourceBytes, err := json.Marshal(source)
		if err != nil {
			return errors.ErrorfWithCause(
				err,
				"failed to marshal source %#v to JSON: %v",
				source, err)
		}
		r = bytes.NewBuffer(sourceBytes)
	}
	var b *bytes.Buffer
	// using io.Writer so if we don't have to allocate a buffer, then
	// the interface itself will be nil.
	var w io.Writer
	if target != nil {
		b = new(bytes.Buffer)
		w = b
	}
	options = append(options, setContentType("application/json"))
	if err := c.request(method, uri, r, w, options...); err != nil {
		return err
	}
	if b != nil {
		return json.Unmarshal(b.Bytes(), target)
	}
	return nil
}

func (c *Client) requestJSONURL(method string, uri *url.URL, source, target interface{}) error {
	if uri == nil {
		return errors.Errorf("uri cannot be nil")
	}
	uri2 := *uri
	c.setAbsURL(&uri2)
	return c.requestJSON(method, uri2.String(), source, target)
}

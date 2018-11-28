package web_test

import (
	"bytes"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/skillian/errors"
	"github.com/skillian/sharebase/web"
)

type Config struct {
	DataCenter    string `json:"dataCenter"`
	Token         string `json:"token"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	SmallTestFile string `json:"smallTestFile"`
	LargeTestFile string `json:"largeTestFile"`
}

var (
	config       Config
	authToken    web.AuthToken
	clientPool   *web.ClientPool = web.NewClientPool()
	expectedData []byte
	expectedHash [sha512.Size]byte
)

const (
	configFilename = "/home/sean/Dropbox/dev/go/src/github.com/skillian/sharebase/web/config.json"

	libraryName  = "My Library"
	folderName   = "Test"
	documentName = "test_data.bin"
)

func init() {
	f, err := os.Open(configFilename)
	if err != nil {
		panic(errors.ErrorfWithCause(err, "failed to open %q: %v", configFilename, err))
	}
	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		panic(errors.ErrorfWithCause(err, "failed to read all of %q: %v", configFilename, err))
	}
	err = json.Unmarshal(bytes, &config)
	if err != nil {
		panic(errors.ErrorfWithCause(err, "failed to unmarshal %q as JSON: %v", configFilename, err))
	}

	authToken, err = web.AuthTokenForUsernameAndPassword(
		config.DataCenter,
		config.Username,
		config.Password)

	if err != nil {
		panic(errPlusConfig("authenticating", err))
	}

	testFile, err := os.Open(config.SmallTestFile)
	if err != nil {
		panic(errPlusConfig(fmt.Sprintf("opening %q", config.SmallTestFile), err))
	}
	defer testFile.Close()

	expectedData, err = ioutil.ReadAll(testFile)
	if err != nil {
		panic(errPlusConfig(fmt.Sprintf("reading all of %q", config.SmallTestFile), err))
	}

	expectedHash = sha512.Sum512(expectedData)
}

func mustGetClient() *web.Client {
	c, err := clientPool.Client(config.DataCenter, authToken.Token)
	if err != nil {
		panic(errPlusConfig("getting a client from the pool", err))
	}
	return c
}

// errPlusConfig includes information about the configuration in the error
// message.
func errPlusConfig(what string, err error) error {
	return errors.ErrorfWithCause(
		err,
		"failure %v with data center: %q, username: %v: %v",
		what, config.DataCenter, config.Username, err)
}

func TestClientLibraries(t *testing.T) {
	t.Parallel()

	c := mustGetClient()
	defer clientPool.Cache(c)

	libs, err := c.Libraries()
	if err != nil {
		t.Fatal(errors.ErrorfWithCause(err, "failed to get libraries: %v", err))
	}
	t.Logf("Libraries: %#v", libs)
}

func TestDownload(t *testing.T) {
	t.Parallel()

	c := mustGetClient()
	defer clientPool.Cache(c)

	lib, err := c.LibraryByName(libraryName)
	if err != nil {
		t.Fatal(errPlusConfig(fmt.Sprintf(
			"finding %q library", libraryName), err))
	}

	fld, err := lib.FolderByName(c, folderName)
	if err != nil {
		t.Fatal(errPlusConfig(fmt.Sprintf(
			"finding %q folder in %q library", folderName, libraryName), err))
	}

	doc, err := fld.DocumentByName(c, documentName)
	if err != nil {
		t.Fatal(errPlusConfig(fmt.Sprintf(
			"finding %q document in %q folder",
			documentName, folderName), err))
	}

	content, err := doc.Content(c)
	if err != nil {
		t.Fatal(errPlusConfig(fmt.Sprintf(
			"getting content from %v", doc), err))
	}
	defer content.Close()

	if content.Length > 1<<20 {
		t.Fatal(doc, "content cannot be >", 1<<20)
	}

	t.Log(doc,
		"content length:", content.Length,
		"content type:", content.ContentType,
		"content disposition:", content.ContentDisposition)

	buf := make([]byte, content.Length)
	for i := 0; i < content.Length; {
		n, err := content.Read(buf[i:])
		i += n
		if err != nil {
			if err == io.EOF {
				continue
			}
			t.Fatal(err)
		}

	}
	//t.Log(expectedData, "\n\n", buf, "\n\n")

	contentHash := sha512.Sum512(buf)
	if len(contentHash) != len(expectedHash) {
		t.Fatalf(
			"length of content's hash (%d) not equal to the length of the expected hash (%d)",
			len(contentHash), len(expectedHash))
	}

	if contentHash != expectedHash {
		t.Fatalf(
			"%v content hash didn't match expected hash:\n\ncontent:\n\n%v\n\nexpected:\n\n%v\n",
			doc, contentHash, expectedHash)
	}

	t.Log("OK")
}

func TestUploadSmall(t *testing.T) {
	t.Parallel()

	filename := strings.Join([]string{
		"test_data_",
		time.Now().Format("2006-01-02_15-04-05"),
		".bin",
	}, "")

	c := mustGetClient()
	defer clientPool.Cache(c)

	lib, err := c.LibraryByName(libraryName)
	if err != nil {
		t.Fatal(errPlusConfig("getting library: "+libraryName, err))
	}

	fld, err := lib.FolderByName(c, folderName)
	if err != nil {
		t.Fatal(errPlusConfig("getting folder: "+folderName, err))
	}

	err = fld.NewDocument(c, filename, bytes.NewReader(expectedData))
	if err != nil {
		t.Fatal(errPlusConfig("uploading "+filename, err))
	}
}

func TestUploadLarge(t *testing.T) {
	t.Parallel()

	c := mustGetClient()
	defer clientPool.Cache(c)

	large, err := os.Open(config.LargeTestFile)
	if err != nil {
		t.Fatal(errPlusConfig("opening "+config.LargeTestFile, err))
	}
	defer func() {
		if err2 := large.Close(); err2 != nil {
			t.Fatal(errPlusConfig("closing "+config.LargeTestFile, err2))
		}
	}()

	lib, err := c.LibraryByName(libraryName)
	if err != nil {
		t.Fatal(errPlusConfig("getting library: "+libraryName, err))
	}

	fld, err := lib.FolderByName(c, folderName)
	if err != nil {
		t.Fatal(errPlusConfig("getting folder: "+folderName, err))
	}

	filename := path.Base(large.Name())
	err = fld.NewDocument(c, filename, large)
	if err != nil {
		t.Fatal(errPlusConfig("uploading "+filename, err))
	}
}

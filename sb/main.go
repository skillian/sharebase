package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/skillian/sharebase/web"

	"github.com/skillian/errors"
	"github.com/skillian/logging"
)

const (
	ShareBaseSchemePrefix = "sb:"
)

var (
	logger = logging.GetLogger("github.com/skillian/sharebase")

	// stdinURL is a placeholder URL for standard input.
	stdinURL = new(url.URL)

	// stdoutURL is a placeholder URL for standard output.""
	stdoutURL = new(url.URL)
)

func init() {
	h := new(logging.ConsoleHandler)
	h.SetLevel(logging.DebugLevel)
	h.SetFormatter(logging.DefaultFormatter{})
	logger.AddHandler(h)
}

// main is responsible for parsing the command line arguments and running
// the Run command
func main() {
	p := Parameters{}

	flag.StringVar(
		&p.Config, "Config", "",
		"Combined configuration file")
	flag.StringVar(
		&p.DataCenter, "DataCenter", "https://app.sharebase.com/sharebaseapi",
		"The ShareBase data center to connect to.\n")
	flag.StringVar(
		&p.Username, "Username", "",
		"The username used to establish a connection to the data center.")
	flag.StringVar(
		&p.Password, "Password", "",
		"The password used to connect to ShareBase.")
	flag.StringVar(
		&p.Token, "Token", "",
		"The ShareBase authentication token included in all requests.  Using a token\n"+
			"precludes the use of a username and password and vice-versa.")

	logLevelString := flag.String("LogLevel", "Warn", "Logging level")

	defaultUsage := flag.Usage

	flag.Usage = func() {
		defaultUsage()
		fmt.Printf(`
Positional parameters:
  [Source] string
        The source file to read from.  Can be either a ShareBase URL or a local
        path.
  [Target] string
        The target file to write to.  Can be either a ShareBase URL or a local
        path.

All of the variables can reference a configuration file instead of an actual
value by prefixing the parameter value with an @.  For example, if the password
is in a text file, the password can be specified as:

    -Password @password_file.txt

To load the password from a file called "password_file.txt" instead of putting
the actual value on the command line.

If a single configuration file is used with the Config parameter, the parameters
within the file must each be on their own line and prefixed with the command
line argument (e.g. "DataCenter", etc.), then an equals sign ("=") and then the
parameter value, for example:

    DataCenter = https://app.sharebase.com/sharebaseapi
    Username   = MyUsername@email.com
    Password   = My Sup3r S#cret P@ssw0rd

`)
	}

	flag.Parse()

	if logLevel, ok := logging.ParseLevel(*logLevelString); ok {
		logger.SetLevel(logLevel)
	}

	args := flag.Args()

	switch len(args) {
	case 0:
		die(errors.Errorf("Source must be specified"))
	case 1:
		p.SourceName = args[0]
		p.TargetName = path.Base(p.SourceName)
	case 2:
		p.SourceName = args[0]
		p.TargetName = args[1]
	default:
		die(errors.Errorf("Too many additional arguments specified!"))
	}

	if err := p.Main(); err != nil {
		die(err)
	}
}

func dieOnError(f func() error) {
	if err := f(); err != nil {
		die(err)
	}
}

func die(err error) {
	logger.LogErr(err)
	os.Exit(-1)
}

// Parameters is a collection of ShareBase parameters.
type Parameters struct {
	web.Client
	state
	Config     string
	DataCenter string
	Username   string
	Password   string
	Token      string

	// SourceName is the name of the source specified on the command line.
	SourceName string

	// TargetName is the name of the source specified on the command line.
	TargetName string
}

// Init canonicalizes the Parameters.
func (p *Parameters) Init() (err error) {
	// The functions called by Init are expected to use the
	// github.com/skillian/errors package to produce full error stack traces.
	if err = p.initConfig(); err != nil {
		return
	}
	if err = p.initDataCenter(); err != nil {
		return
	}
	if err = p.initUserPass(); err != nil {
		return
	}
	if err = p.initToken(); err != nil {
		return
	}
	client, err := web.NewClient(p.DataCenter, p.Token)
	if err != nil {
		return
	}
	p.Client = *client
	closer, err := getIO(&p.Client, readingOp, p.SourceName, "")
	if err != nil {
		return
	}
	p.source = closer.(io.ReadCloser)
	closer, err = getIO(&p.Client, writingOp, p.TargetName, path.Base(p.SourceName))
	if err != nil {
		return
	}
	p.target = closer.(io.WriteCloser)
	return
}

// Main is an indirection from the main function so that errors from deferred
// functions are easily captured.
func (p *Parameters) Main() (err error) {
	if err = p.Init(); err != nil {
		die(errors.ErrorfWithCause(
			err, "failed to initialize parameters: %v", err))
	}

	defer errors.WrapDeferred(&err, p.source.Close)
	defer errors.WrapDeferred(&err, p.target.Close)

	_, err = io.Copy(p.target, p.source)
	return err
}

func (p *Parameters) initConfig() (err error) {
	p.Config, err = GetStringOf(p.Config)
	if err != nil {
		return errors.ErrorfWithCause(
			err, "failed to read config: %v:\n%v", p.Config, err)
	}
	if p.Config == "" {
		return nil
	}
	config := strings.Split(p.Config, "\n")
	params := make([][2]string, 0, len(config))
	for _, line := range config {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		index := strings.Index(line, "=")
		if index == -1 {
			return errors.Errorf(
				"no equals sign in configuration line: %v", line)
		}
		params = append(params, [...]string{
			strings.TrimSpace(line[:index]),
			strings.TrimSpace(line[index+len("="):]),
		})
	}
	// using reflection here in case the field names change in the future:
	v := reflect.ValueOf(p)
	for _, param := range params {
		f := v.Elem().FieldByName(param[0])
		if !f.IsValid() {
			return errors.Errorf("unrecognized field: %q", param[0])
		}
		f.SetString(param[1])
	}
	return nil
}

func (p *Parameters) initDataCenter() (err error) {
	p.DataCenter, err = GetStringOf(p.DataCenter)
	return
}

func (p *Parameters) initUserPass() (err error) {
	p.Username, err = GetStringOf(p.Username)
	if err != nil {
		return errors.ErrorfWithCause(
			err, "failed to load %v from parameters: %v", "Username", err)
	}
	p.Password, err = GetStringOf(p.Password)
	return err
}

// initToken expects the DataCenter to be initialized.
func (p *Parameters) initToken() (err error) {
	p.Token, err = GetStringOf(p.Token)
	if err != nil {
		return err
	}
	if p.Token == "" {
		authToken, err := web.AuthTokenForUsernameAndPassword(
			p.DataCenter, p.Username, p.Password)
		if err != nil {
			return err
		}
		p.Token = authToken.Token
	}
	return nil
}

// GetStringOf gets a string from a given parameter value.  If the value starts
// with an "@", then the value is treated as a filename and opened and read
// to get a string value.  If the value doesn't start with an "@", then the
// value itself is returned.
func GetStringOf(p string) (v string, err error) {
	if len(p) == 0 {
		return "", nil
	}
	if p[0] == '@' {
		file, err := os.Open(p[1:])
		if err != nil {
			return "", errors.ErrorfWithCause(
				err, "failed to open %q for reading: %v", p, err)
		}
		defer errors.WrapDeferred(&err, file.Close)
		var b []byte
		b, err = ioutil.ReadAll(file)
		if err != nil {
			return "", errors.ErrorfWithCause(
				err, "failed to read file %q: %v", file.Name(), err)
		}
		return string(b), nil
	}
	return p, nil
}

// getURL converts a string to a URL.  If the string fails to parse as a URL,
// "file:" is prefixed to it and the result is re-parsed.
func getURL(v string) (uri *url.URL, err error) {
	uri, err = url.Parse(v)
	if err == nil {
		return
	}
	if urlerr, ok := err.(*url.Error); ok {
		if urlerr.Op == "parse" {
			return url.Parse("file:" + v)
		}
	}
	return nil, err
}

type state struct {
	source io.ReadCloser
	target io.WriteCloser
}

type oper int

const (
	invalidOp oper = iota
	readingOp
	writingOp
)

func getIO(c *web.Client, op oper, v, defaultName string) (io.Closer, error) {
	if v == "-" {
		// piping from/to stdin/stdout:
		switch op {
		case readingOp:
			return ioutil.NopCloser(os.Stdin), nil
		case writingOp:
			return ioutil.NopCloser(os.Stdout), nil
		default:
			panic(errors.Errorf("invalid operation: %v", op))
		}
	}
	if strings.HasPrefix(v, ShareBaseSchemePrefix) {
		// is a ShareBase source or target:
		f, name, err := getSBFolderAndDocumentName(c, v[len(ShareBaseSchemePrefix):])
		if err != nil {
			return nil, err
		}
		if name == "" {
			name = defaultName
		}
		if name == "" {
			return nil, errors.Errorf(
				"document or file name cannot be empty")
		}
		switch op {
		case readingOp:
			d, err := f.DocumentByName(c, name)
			if err != nil {
				return nil, err
			}
			return d.Content(c)
		case writingOp:
			return f.DocumentWriter(c, name)
		default:
			panic(errors.Errorf("invalid operation: %v", op))
		}
	}
	switch op {
	case readingOp:
		return os.Open(v)
	case writingOp:
		if fi, err := os.Stat(v); !os.IsNotExist(err) {
			if fi.IsDir() {
				v = path.Join(v, defaultName)
			}
			logger.Warn1("%q will be overwritten", v)
		}
		return os.Create(v)
	default:
		panic(errors.Errorf("invalid operation: %v", op))
	}
}

func getSBFolderAndDocumentName(c *web.Client, pathString string) (f web.Folder, name string, err error) {
	pathParts := strings.Split(pathString, "/")
	if len(pathParts) < 2 {
		err = errors.Errorf(
			"invalid ShareBase path: %v", pathString)
		return
	}
	lib, err := c.LibraryByName(pathParts[0])
	if err != nil {
		return
	}
	f, err = lib.FolderByName(c, pathParts[1])
	if err != nil {
		return
	}
	lastIndex := len(pathParts) - 1
	if lastIndex >= 2 {
		for _, name = range pathParts[2:lastIndex] {
			f, err = f.FolderByName(c, name)
			if err != nil {
				return
			}
		}
	}

	lastName := pathParts[lastIndex]

	f2, err := f.FolderByName(c, lastName)
	if _, ok := err.(web.NotFound); ok {
		// The last part must be the name of a document.
		return f, lastName, nil
	}

	// else f2 is a folder that exists and the new name should be the same name
	// as the source.
	return f2, "", nil
}

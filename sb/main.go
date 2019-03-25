package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/skillian/sharebase/web"

	"github.com/skillian/errors"
	"github.com/skillian/logging"
)

const (
	defaultConfigFilename = ".sharebase-config.json"

	shareBaseURIScheme = "sb:"
)

var (
	programName = path.Base(os.Args[0])

	my = mustGetCurrentUser()

	defaultUsage = flag.Usage

	logger = logging.GetLogger("github.com/skillian/sharebase")
)

func init() {
	h := new(logging.ConsoleHandler)
	h.SetLevel(math.MinInt32)
	h.SetFormatter(logging.DefaultFormatter{})
	logger.AddHandler(h)
}

func main() {
	s := state{}

	var configFilename, logLevelString string

	flag.StringVar(
		&configFilename, "c",
		filepath.Join(my.HomeDir, defaultConfigFilename),
		"ShareBase configuration file")

	flag.StringVar(
		&logLevelString, "l", "",
		"Logging level (useful for debugging)")

	flag.BoolVar(
		&s.Tar, "t", false,
		"Write the source file(s) out into a tar instead of "+
			"individual files (useful for piping them to another "+
			"command).")

	flag.BoolVar(
		&s.Untar, "u", false,
		"Untar the input to write the files separately into "+
			"ShareBase (useful if the source is coming from a "+
			"stream).")

	flag.BoolVar(
		&s.Exec, "x", false,
		"The [source] parameter is a command to execute instead of "+
			"a source file/directory.")

	flag.Usage = func() {
		defaultUsage()
		fmt.Printf(`
Positional parameters:
  [source] string
        The source file to read from.  Can be either a ShareBase URL or a local
        path.
  [target] string
        The target file to write to.  Can be either a ShareBase URL or a local
	path.
`)
	}

	flag.Parse()

	args := flag.Args()

	switch len(args) {
	case 0:
		die(errors.Errorf("Source must be specified"))
	case 1:
		s.Source = args[0]
		s.Target = path.Base(s.Source)
	case 2:
		s.Source = args[0]
		s.Target = args[1]
	default:
		die(errors.Errorf("Too many arguments specified!"))
	}

	dieOnError(loadJSONConfig(configFilename, &s.Config))

	if level, ok := logging.ParseLevel(logLevelString); ok {
		logger.SetLevel(level)
	}

	dieOnError(s.execute())
}

// Config is configuration that is specified inside of a JSON file.
type Config struct {
	DataCenter string `json:"dataCenter"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Token      string `json:"token"`
}

type state struct {
	*web.ClientPool
	*Root
	Config

	Overwrite bool

	Source string
	Target string

	Tar   bool
	Untar bool
	Exec  bool
}

func (s *state) client() (*web.Client, error) {
	return s.ClientPool.Client(
		s.Config.DataCenter, s.Config.Token)
}

func (s *state) init() error {
	if s.Config.Username != "" {
		logger.Debug1(
			"Config w/ username %q specified.  Creating token...",
			s.Config.Username)
		tok, err := web.AuthTokenForUsernameAndPassword(
			s.Config.DataCenter, s.Username, s.Password)
		if err != nil {
			return errors.ErrorfWithCause(
				err,
				"failed to create ShareBase authorization "+
					"token for username: %v: %v",
				s.Config.Username, err)
		}
		logger.Debug2(
			"Token for username %q: %v",
			s.Config.Username, tok.Token)
		s.Config.Token = tok.Token
	}
	s.ClientPool = web.NewClientPool()
	s.Root = NewRoot()
	return nil
}

func (s *state) execute() error {
	var err error
	if err = s.init(); err != nil {
		return err
	}
	c, err := s.client()
	if err != nil {
		return err
	}
	if s.Exec {
		if !isShareBaseLoc(s.Target) {
			return errors.Errorf(
				"commands must execute on ShareBase objects.")
		}
		p := ShareBasePathFromString(s.Target)
		o, err := s.Root.ObjectByPath(c, nil, p)
		if err != nil {
			return errors.ErrorfWithCause(
				err,
				"failed to get %v", p)
		}
		return s.execCommand(c, o)
	}
	if isShareBaseLoc(s.Source) {
		if isShareBaseLoc(s.Target) {
			return errors.Errorf(
				"ShareBase -> ShareBase is not yet supported.")
		}
		if s.Untar {
			return errors.Errorf(
				"cannot untar from ShareBase source.")
		}
		p, base, err := s.Root.ParentByPath(c, nil, ShareBasePathFromString(s.Source))
		if err != nil {
			return err
		}
		return s.shareBaseToLocal(c, p, base)
	}
	if isShareBaseLoc(s.Target) {
		if isShareBaseLoc(s.Source) {
			return errors.Errorf(
				"ShareBase -> ShareBase is not yet supported.")
		}
		if s.Tar {
			return errors.Errorf("cannot tar to ShareBase target.")
		}
		path := ShareBasePathFromString(s.Target)
		logger.Debug("Path: %v", path)
		p, base, err := s.Root.ParentByPath(c, nil, path)
		if err != nil {
			return err
		}
		return s.localToShareBase(c, p, base)
	}
	return errors.Errorf(
		"do not use this client for local -> local transfers.")
}

func (s *state) execCommand(c *web.Client, o Object) error {
	cmd := strings.ToLower(s.Source)
	fn, ok := commands[cmd]
	if !ok {
		return errors.Errorf("unrecognized command: %q", cmd)
	}
	return fn(s, c, o)
}

var commands = map[string]func(s *state, c *web.Client, o Object) error{
	"hash": (*state).hashDocument,
	"ls":   (*state).listDirectory,
}

func (s *state) hashDocument(c *web.Client, o Object) error {
	d, ok := o.(*Document)
	if !ok {
		return errors.Errorf(
			"only %T can be hashed, not %T", d, o)
	}
	return writeObjectToList(d, os.Stdout)
}

func (s *state) listDirectory(c *web.Client, o Object) error {
	p, ok := o.(Parent)
	if !ok {
		return errors.Errorf(
			"%v is not a parent (it's a %T)", PathOf(p), p)
	}
	if err := p.update(s.Root, c); err != nil {
		return errors.ErrorfWithCause(
			err,
			"failed to update ShareBase Folder %v", PathOf(p))
	}
	for _, ch := range p.Children() {
		if err := writeObjectToList(ch, os.Stdout); err != nil {
			return err
		}
	}
	return nil
}

func writeObjectToList(o Object, w io.Writer) error {
	var err error
	t := reflect.TypeOf(o)
	switch o := o.(type) {
	case *Document:
		_, err = fmt.Fprintf(
			w,
			"%s\tID: %d\t%s\t%s\t%s\n",
			o.Name(),
			o.ID(),
			t.Name(),
			o.DateModified.Format("2006-01-02 03:04:05 PM EST"),
			getHex(o.Hash))
		//	case *Folder:
		//		_, err = fmt.Fprintf(
		//			w, "%s\tID: %d\t%s\n", o.Name(), o.ID(), t.Name())
	default:
		_, err = fmt.Fprintf(
			w, "%s\tID: %d\t%s\n", o.Name(), o.ID(), t.Name())
	}
	return err
}

func (s *state) localToShareBase(wc *web.Client, p Parent, name string) (err error) {
	//logger.Debug2("parent: %v, name: %q", PathOf(p), name)
	var source *os.File
	if s.Source == "" || s.Source == "-" {
		source = os.Stdin
	} else {
		source, err = os.Open(s.Source)
		if err != nil {
			return err
		}
		defer errors.WrapDeferred(&err, source.Close)
	}
	st, err := source.Stat()
	if err != nil {
		return errors.Errorf(
			"failed to stat source: %v: %v", s.Source, err)
	}
	if source != os.Stdin {
		// Don't try the if-dir-exists behavior if we're reading
		// from stdin.
		if c, err := s.Root.ObjectByPath(wc, p, ShareBasePathFromString(name)); err == nil {
			logger.Debug2("parent %q has child %q", PathOf(p), name)
			if c, ok := c.(Parent); ok {
				logger.Debug1("child %q is itself a parent", PathOf(c))
				p = c
				name = path.Base(source.Name())
			}
		}
	}
	if st.IsDir() {
		if s.Untar {
			return errors.Errorf(
				"cannot untar source directory: %v",
				s.Source)
		}
		return s.localDirToShareBaseDir(wc, source, p, name)
	}
	if s.Untar {
		return s.localTarToShareBaseDir(wc, source, p, name)
	}
	f, ok := p.(*Folder)
	if !ok {
		return errors.Errorf(
			"files can only be uploaded into folders, not %T", p)
	}
	if err != nil {
		return err
	}
	err = s.localFileToShareBaseDir(wc, source, f, name)
	return err
}

// localDirToShareBaseDir copies a local directory into a ShareBase directory.
//
// Currently, it uses recursion, so this could be a problem for very deep
// folder structures.
func (s *state) localDirToShareBaseDir(wc *web.Client, source *os.File, p Parent, name string) error {
	logger.Debug2("parent: %v, name: %q", PathOf(p), name)
	f, err := s.Root.GetOrCreateFolder(wc, p, ShareBasePathFromString(name))
	if err != nil {
		return errors.ErrorfWithCause(
			err, "failed to create ShareBase folder")
	}
	for {
		infos, err := source.Readdir(128)
		if err != nil && err != io.EOF {
			return errors.ErrorfWithCause(
				err,
				"failed to read local directory: %v: %v:",
				source.Name(), err)
		}
		for _, fi := range infos {
			name := fi.Name()
			file, err := os.Open(path.Join(source.Name(), name))
			name = path.Base(name)
			if err != nil {
				return err
			}
			if fi.IsDir() {
				err = s.localDirToShareBaseDir(wc, file, f, name)
			} else {
				err = s.localFileToShareBaseDir(wc, file, f, name)
			}
			if err != nil {
				return err
			}
			if err = file.Close(); err != nil {
				return err
			}
		}
		if err == io.EOF {
			break
		}
	}
	return nil
}

func (s *state) localFileToShareBaseDir(c *web.Client, r io.Reader, f *Folder, name string) error {
	logger.Info2("copying %v to %v...", name, PathOf(f))
	// Don't need to worry about updating Root.  It'll find out about the
	// new document the next time it's refreshed.  No need to rack up
	// possibly unecessary requests.  Plus, we don't know what the
	// new doc's ID is without re-requesting from the API.
	return f.Folder.NewDocument(c, name, r)
}

func (s *state) localTarToShareBaseDir(wc *web.Client, r io.Reader, origin Parent, name string) error {
	f, err := s.Root.GetOrCreateFolder(wc, origin, ShareBasePathFromString(name))
	if err != nil {
		return errors.ErrorfWithCause(
			err, "failed to create ShareBase folder")
	}
	t := tar.NewReader(r)
	for {
		h, err := t.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return errors.ErrorfWithCause(
				err, "failure while reading tar")
		}
		switch h.Typeflag {
		case tar.TypeDir:
			_, err = s.Root.GetOrCreateFolder(wc, f, LocalPathFromString(h.Name))
			if err != nil {
				return errors.ErrorfWithCause(
					err,
					"failed to create subdirectory")
			}
		case tar.TypeReg:
			path := LocalPathFromString(h.Name)
			f2, err := s.Root.GetOrCreateFolder(wc, f, path.Dir())
			if err != nil {
				return errors.ErrorfWithCause(
					err,
					"failed to get target directory %v: %v",
					path.Dir(), err)
			}
			err = s.localFileToShareBaseDir(wc, t, f2, Basename(path))
			if err != nil {
				return errors.ErrorfWithCause(
					err,
					"failed to write contents of tar "+
						"into ShareBase Document: %v",
					err)
			}
		}
	}
	return nil
}

func (s *state) shareBaseToLocal(wc *web.Client, p Parent, name string) (err error) {
	var o Object
	o, err = s.Root.ObjectByPath(wc, p, ShareBasePathFromString(name))
	if err != nil {
		return errors.ErrorfWithCause(
			err,
			"failed to get source ShareBase document or folder.")
	}
	p2, ok := o.(Parent)
	target, err := s.getLocalTarget(ok && !s.Tar, name)
	if err != nil {
		return err
	}
	defer errors.WrapDeferred(&err, target.Close)
	if ok {
		if s.Tar {
			return s.shareBaseDirToLocalTar(wc, p2)
		}
		return s.shareBaseDirToLocalDir(wc, p2, LocalPathFromString(target.Name()))
	}
	return s.shareBaseFileToLocalFile(wc, o, target)
}

func (s *state) shareBaseDirToLocalDir(wc *web.Client, p Parent, target LocalPath) error {
	stat, err := os.Stat(target.String())
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.Mkdir(s.Target, 0777); err != nil {
				return errors.ErrorfWithCause(
					err,
					"failed to create target directory "+
						"%q: %v",
					s.Target, err)
			}
		} else {
			return errors.ErrorfWithCause(
				err,
				"failed to create directory %q: %v",
				target, err)
		}
	}
	if !stat.IsDir() {
		return errors.Errorf(
			"target location %q exists but is not a directory",
			target)
	}
	return errors.Errorf("not implemented")
}

func (s *state) shareBaseDirToLocalTar(wc *web.Client, p Parent) error {
	panic("not implemented")
}

func (s *state) shareBaseFileToLocalFile(wc *web.Client, o Object, target *os.File) error {
	panic("not implemented")
}

// getLocalTarget gets the local target file or directory.
func (s *state) getLocalTarget(container bool, name string) (*os.File, error) {
	if s.Target == "" || s.Target == "-" {
		return os.Stdout, nil
	}
	st, err := os.Stat(s.Target)
	if err != nil {
		if os.IsNotExist(err) {
			if container {
				if err = os.Mkdir(s.Target, 0777); err != nil {
					return nil, errors.ErrorfWithCause(
						err,
						"failed to create target directory "+
							"%q: %v",
						s.Target, err)
				}
				return os.Open(s.Target)
			}
			return os.Create(s.Target)
		}
		return nil, errors.ErrorfWithCause(
			err,
			"failed to get statistics on file %q: %v",
			s.Target, err)
	}
	if st.IsDir() {
		s.Target = path.Join(s.Target, name)
		st, err = os.Stat(s.Target)
		if err != nil && !os.IsNotExist(err) {
			return nil, errors.ErrorfWithCause(
				err,
				"failed to stat %q: %v", s.Target, err)
		}
	}
	if !s.Overwrite && (err == nil || os.IsExist(err)) {
		return nil, errors.ErrorfWithCause(
			err,
			"refusing to overwrite existing target %q")
	}
	return os.Create(s.Target)
}

// die reports the given error and terminates the program with a non-0 return
// code.
func die(err error) {
	logger.LogErr(err)
	os.Exit(-1)
}

// dieOnError calls die if the given error is not nil.
func dieOnError(err error) {
	if err != nil {
		die(err)
	}
}

func isShareBaseLoc(v string) bool {
	return strings.HasPrefix(v, shareBaseURIScheme)
}

func loadJSONConfig(filename string, c *Config) (err error) {
	const loadJSONConfigErrFmt = "failed to %v JSON configuration file: %v"
	f, err := os.Open(filename)
	if err != nil {
		return errors.ErrorfWithCause(
			err,
			loadJSONConfigErrFmt, "open", err)
	}
	defer errors.WrapDeferred(&err, f.Close)
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return errors.ErrorfWithCause(
			err,
			loadJSONConfigErrFmt, "read", err)
	}
	if err = json.Unmarshal(data, c); err != nil {
		return errors.ErrorfWithCause(
			err,
			loadJSONConfigErrFmt, "parse", err)
	}
	return nil
}

func mustGetCurrentUser() *user.User {
	u, err := user.Current()
	if err != nil {
		panic(errors.ErrorfWithCause(
			err,
			"failed to get current user: %v", err))
	}
	return u
}

func getHex(bs []byte) string {
	var buffer bytes.Buffer
	for _, b := range bs {
		//		buffer.WriteString(fmt.Sprintf("%02x", b))
		if _, err := fmt.Fprintf(&buffer, "%02x", b); err != nil {
			panic(errors.ErrorfWithCause(
				err,
				"failed to print %#v with format %%02x", b))
		}
	}
	return buffer.String()
}

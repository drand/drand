package main

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/nikkolasg/slog"
)

// Store abstracts the loading and saving of any configuration/cryptographic
// material to be used by drand. For the moment, only a file based store is
// implemented.
type Store interface {
	SaveKey(p *Private) error
	LoadKey() (*Private, error)
	LoadGroup() (*Group, error)
	SaveShare(share *Share) error
	LoadShare() (*Share, error)
	SaveSignature(b *BeaconSignature) error
	LoadSignature(timestamp int64) (*BeaconSignature, error)
	SignatureExists(timestamp int64) bool
}

var ErrStoreFile = errors.New("store file issues")
var ErrAbsent = errors.New("store can't find requested object")

// defaultDataFolder is the default place where the secret keys, and signatures
// will be stored.
const defaultDataFolder = ".drand"
const defaultKeyFile = "drand_id"
const privateExtension = ".private"
const publicExtension = ".public"
const defaultGroupFile_ = "drand_group"
const groupExtension = ".toml"
const shareExtension = ".secret"
const defaultSigFolder_ = "beacons"

const keyFileFlagName = "key"
const groupFileFlagName = "group"
const sigFolderFlagName = "beacons"

// Tomler represents any struct that can be (un)marshalled into/from toml format
type Tomler interface {
	TOML() interface{}
	FromTOML(i interface{}) error
	TOMLValue() interface{}
}

// FileStore is a FileStore using filesystem to store informations
type FileStore struct {
	KeyFile    string
	PublicFile string
	GroupFile  string
	ShareFile  string
	SigFolder  string
}

func DefaultFileStore() *FileStore {
	return &FileStore{
		KeyFile:    defaultPrivateFile(),
		PublicFile: publicFile(defaultPrivateFile()),
		GroupFile:  defaultGroupFile(),
		ShareFile:  shareFile(defaultGroupFile()),
		SigFolder:  defaultSigFolder(),
	}
}

// KeyValue is a store that returns a value under a key. It must returns a
// default value in case the key is not defined. Keys are defined above as
// XXXFlagName.
// Initially, cli.Context only fulfills this role but it's easy to imagine other
// implementations in the future (change of cli-framework or else).
type KeyValue interface {
	String(key string) string
}

func NewFileStore(k KeyValue) *FileStore {
	return &FileStore{
		KeyFile:    k.String(keyFileFlagName),
		PublicFile: publicFile(k.String(keyFileFlagName)),
		GroupFile:  k.String(groupFileFlagName),
		ShareFile:  shareFile(k.String(groupFileFlagName)),
		SigFolder:  k.String(sigFolderFlagName),
	}
}

// SaveKey first saves the private key in a file with tight permissions and then
// saves the public part in another file.
func (f *FileStore) SaveKey(p *Private) error {
	if err := f.Save(f.KeyFile, p, true); err != nil {
		return err
	}
	return f.Save(f.PublicFile, p.Public, false)
}

// LoadKey decode private key first then public
func (f *FileStore) LoadKey() (*Private, error) {
	p := new(Private)
	if err := f.Load(f.KeyFile, p); err != nil {
		return nil, err
	}
	return p, f.Load(f.PublicFile, p.Public)
}

func (f *FileStore) LoadGroup() (*Group, error) {
	g := new(Group)
	return g, f.Load(f.GroupFile, g)
}

func (f *FileStore) SaveShare(share *Share) error {
	return f.Save(f.ShareFile, share, true)
}

func (f *FileStore) LoadShare() (*Share, error) {
	s := new(Share)
	return s, f.Load(f.ShareFile, s)
}

func (f *FileStore) SaveSignature(b *BeaconSignature) error {
	os.MkdirAll(f.SigFolder, os.ModePerm)
	return f.Save(f.beaconFilename(b.Request.Timestamp), b, true)
}

func (f *FileStore) LoadSignature(ts int64) (*BeaconSignature, error) {
	fname := f.beaconFilename(ts)
	sig := new(BeaconSignature)
	return sig, f.Load(fname, sig)
}

func (f *FileStore) SignatureExists(ts int64) bool {
	ok, _ := exists(f.beaconFilename(ts))
	return ok
}

func (f *FileStore) Save(path string, t Tomler, secure bool) error {
	var fd *os.File
	var err error
	if secure {
		fd, err = createSecureFile(path)
	} else {
		fd, err = os.Create(path)
	}
	if err != nil {
		slog.Infof("config: can't save %s to %s: %s", reflect.TypeOf(t).String(), path, err)
		return nil
	}
	defer fd.Close()
	return toml.NewEncoder(fd).Encode(t.TOML())
}

func (f *FileStore) Load(path string, t Tomler) error {
	tomlValue := t.TOMLValue()
	if _, err := toml.DecodeFile(path, tomlValue); err != nil {
		return err
	}
	return t.FromTOML(tomlValue)
}

// toFilename returns the filename where a signature having the given timestamp
// is stored.
func (f *FileStore) beaconFilename(ts int64) string {
	return path.Join(f.SigFolder, fmt.Sprintf("%s.sig", ts))
}

// default threshold for the distributed key generation protocol & TBLS.
func defaultThreshold(n int) int {
	return n * 2 / 3
}

func defaultPrivateFile() string {
	return path.Join(appData(), defaultKeyFile+privateExtension)
}

// XXX quick hack, probably a thousand ways to abuse this...
func publicFile(privateFile string) string {
	ss := strings.Split(privateFile, privateExtension)
	return ss[0] + publicExtension
}

func defaultGroupFile() string {
	return path.Join(appData(), defaultGroupFile_) + groupExtension
}

// XXX quick hack, probably a thousand ways to abuse this...
func shareFile(groupFile string) string {
	ss := strings.Split(groupFile, groupExtension)
	return ss[0] + shareExtension
}

func defaultSigFolder() string {
	return path.Join(appData(), defaultSigFolder_)
}

// appData returns the directory where drand stores all its information.
// It creates the path if it not existent yet.
func appData() string {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	path := path.Join(u.HomeDir, defaultDataFolder)
	if exists, _ := exists(path); !exists {
		if err := os.MkdirAll(path, 0740); err != nil {
			panic(err)
		}
	}
	return path
}

// pwd returns the current directory. Useless for now.
func pwd() string {
	s, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return s
}

// exists returns whether the given file or directory exists or not
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func flagNameStruct(name string) string {
	return name + " ," + string(name[0])
}

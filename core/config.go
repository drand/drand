package core

import (
	"path"
	"time"

	bolt "github.com/coreos/bbolt"
	"github.com/dedis/drand/beacon"
	"github.com/dedis/drand/dkg"
	"github.com/dedis/drand/fs"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"google.golang.org/grpc"
)

// DefaultConfigFolderName is the name of the folder containing all key materials
// (and the beacons db file by default). It is relative to the user's home
// directory.
const DefaultConfigFolderName = ".drand"

func DefaultConfigFolder() string {
	return path.Join(fs.HomeFolder(), DefaultConfigFolderName)
}

// DefaultDbFolder is the name of the folder in which the db file is saved. By
// default it is relative to the DefaultConfigFolder path.
const DefaultDbFolder = "db"

// DefaultBeaconPeriod is the period in which the beacon logic creates new
// random beacon.
const DefaultBeaconPeriod time.Duration = 1 * time.Minute

// DefaultControlPort is the default port the functionnality control port communicate on.
const DefaultControlPort = "8888"

type ConfigOption func(*Config)

type Config struct {
	configFolder string
	dbFolder     string
	listenAddr   string
	controlPort  string
	grpcOpts     []grpc.DialOption
	callOpts     []grpc.CallOption
	dkgTimeout   time.Duration
	boltOpts     *bolt.Options
	beaconCbs    []func(*beacon.Beacon)
	insecure     bool
	certPath     string
	keyPath      string
	certmanager  *net.CertManager
}

// NewConfig returns the config to pass to drand with the default options set
// and the updated values given by the options.
func NewConfig(opts ...ConfigOption) *Config {
	d := &Config{
		configFolder: DefaultConfigFolder(),
		//grpcOpts:     []grpc.DialOption{grpc.WithInsecure()},
		dkgTimeout:  dkg.DefaultTimeout,
		certmanager: net.NewCertManager(),
		controlPort: DefaultControlPort,
	}
	d.dbFolder = path.Join(d.configFolder, DefaultDbFolder)
	for i := range opts {
		opts[i](d)
	}
	return d
}

func (d *Config) ConfigFolder() string {
	return d.configFolder
}

func (d *Config) DBFolder() string {
	return d.dbFolder
}

func (d *Config) Certs() *net.CertManager {
	return d.certmanager
}

// ListenAddress returns the given default address or the listen address stored
// in the config thanks to WithListenAddress
func (d *Config) ListenAddress(defaultAddr string) string {
	if d.listenAddr != "" {
		return d.listenAddr
	}
	return defaultAddr
}

// ControlPort returns the port used for control port communications
// which can be the default one or the port setup thanks to WithControlPort
func (d *Config) ControlPort() string {
	return d.controlPort
}

func (d *Config) callbacks(b *beacon.Beacon) {
	for _, fn := range d.beaconCbs {
		fn(b)
	}
}

func WithGrpcOptions(opts ...grpc.DialOption) ConfigOption {
	return func(d *Config) {
		d.grpcOpts = opts
	}
}

func WithCallOption(opts ...grpc.CallOption) ConfigOption {
	return func(d *Config) {
		d.callOpts = opts
	}
}

func WithDkgTimeout(t time.Duration) ConfigOption {
	return func(d *Config) {
		d.dkgTimeout = t
	}
}

func WithBoltOptions(opts *bolt.Options) ConfigOption {
	return func(d *Config) {
		d.boltOpts = opts
	}
}

// WithDbFolder sets the path folder for the db file. This path is NOT relative
// to the DrandFolder path if set.
func WithDbFolder(folder string) ConfigOption {
	return func(d *Config) {
		d.dbFolder = folder
	}
}

func WithConfigFolder(folder string) ConfigOption {
	return func(d *Config) {
		d.configFolder = folder
		d.dbFolder = path.Join(d.configFolder, DefaultDbFolder)
	}
}

func WithBeaconCallback(fn func(*beacon.Beacon)) ConfigOption {
	return func(d *Config) {
		d.beaconCbs = append(d.beaconCbs, fn)
	}
}

func WithInsecure() ConfigOption {
	return func(d *Config) {
		d.insecure = true
	}
}

func WithTLS(certPath, keyPath string) ConfigOption {
	return func(d *Config) {
		d.certPath = certPath
		d.keyPath = keyPath
	}
}

func WithTrustedCerts(certPaths ...string) ConfigOption {
	return func(d *Config) {
		for _, p := range certPaths {
			if err := d.certmanager.Add(p); err != nil {
				panic(err)
			}
		}
	}
}

// WithListenAddress specifies the address the drand instance should bind to. It
// is useful if you want to advertise a public proxy address and the drand
// instance runs behind your network.
func WithListenAddress(addr string) ConfigOption {
	return func(d *Config) {
		d.listenAddr = addr
	}
}

// WithControlPort specifies which port on localhost the ListenerControl should bind to.
func WithControlPort(port string) ConfigOption {
	return func(d *Config) {
		d.controlPort = port
	}
}

func getPeriod(g *key.Group) time.Duration {
	if g.Period == time.Duration(0) {
		return DefaultBeaconPeriod
	}
	return g.Period
}

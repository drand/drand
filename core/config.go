package core

import (
	"path"
	"time"

	bolt "github.com/coreos/bbolt"
	"github.com/dedis/drand/core/beacon"
	"github.com/dedis/drand/core/dkg"
	"google.golang.org/grpc"
)

// DefaultConfigFolder is the name of the folder containing all key materials
// (and the beacons db file by default). It is relative to the user's home
// directory.
const DefaultConfigFolder = ".drand"

// DefaultDbFolder is the name of the folder in which the db file is saved. By
// default it is relative to the DefaultConfigFolder path.
const DefaultDbFolder = "db"

// DefaultBeaconPeriod is the period in which the beacon logic creates new
// random beacon.
const DefaultBeaconPeriod time.Duration = 1 * time.Minute

type ConfigOption func(*Config)

type Config struct {
	configFolder string
	dbFolder     string
	listenAddr   string
	grpcOpts     []grpc.DialOption
	dkgTimeout   time.Duration
	boltOpts     *bolt.Options
	beaconPeriod time.Duration
	beaconCbs    []func(*beacon.Beacon)
}

// NewConfig returns the config to pass to drand with the default options set
// and the updated values given by the options.
func NewConfig(opts ...ConfigOption) *Config {
	d := &Config{
		configFolder: DefaultConfigFolder,
		grpcOpts:     []grpc.DialOption{grpc.WithInsecure()},
		dkgTimeout:   dkg.DefaultTimeout,
		beaconPeriod: DefaultBeaconPeriod,
	}
	d.dbFolder = path.Join(DefaultConfigFolder, DefaultDbFolder)
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

// ListenAddress returns the given default address or the listen address stored
// in the config thanks to WithListenAddress
func (d *Config) ListenAddress(defaultAddr string) string {
	if d.listenAddr != "" {
		return d.listenAddr
	}
	return defaultAddr
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
	}
}

func WithBeaconPeriod(period time.Duration) ConfigOption {
	return func(d *Config) {
		d.beaconPeriod = period
	}
}

func WithBeaconCallback(fn func(*beacon.Beacon)) ConfigOption {
	return func(d *Config) {
		d.beaconCbs = append(d.beaconCbs, fn)
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

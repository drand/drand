package core

import (
	"path"
	"time"

	"github.com/benbjohnson/clock"
	bolt "github.com/coreos/bbolt"
	"github.com/drand/drand/beacon"
	"github.com/drand/drand/dkg"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"google.golang.org/grpc"
)

// ConfigOption is a function that applies a specific setting to a Config.
type ConfigOption func(*Config)

// Config holds all relevant information for a drand node to run.
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
	dkgCallback  func(*key.Share)
	insecure     bool
	certPath     string
	keyPath      string
	certmanager  *net.CertManager
	logger       log.Logger
	clock        clock.Clock
}

// NewConfig returns the config to pass to drand with the default options set
// and the updated values given by the options.
func NewConfig(opts ...ConfigOption) *Config {
	d := &Config{
		configFolder: DefaultConfigFolder(),
		//grpcOpts:     []grpc.DialOption{grpc.WithInsecure()},
		grpcOpts: []grpc.DialOption{
			/*grpc.WithBackoffMaxDelay(3 * time.Second),*/
			//grpc.WithBlock(),
			//grpc.FailOnNonTempDialError(true),
			/*grpc.WithTimeout(DefaultDialTimeout),*/
		},
		dkgTimeout: dkg.DefaultTimeout,
		//certmanager: net.NewCertManager(),
		controlPort: DefaultControlPort,
		logger:      log.DefaultLogger,
		clock:       clock.New(),
	}
	d.dbFolder = path.Join(d.configFolder, DefaultDbFolder)
	for i := range opts {
		opts[i](d)
	}
	return d
}

// ConfigFolder returns the folder under which drand stores all its
// configuration.
func (d *Config) ConfigFolder() string {
	return d.configFolder
}

// DBFolder returns the folder under which drand stores all generated beacons.
func (d *Config) DBFolder() string {
	return d.dbFolder
}

// Certs returns all custom certs currently being trusted by drand.
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

// Logger returns the logger associated with this config.
func (d *Config) Logger() log.Logger {
	return d.logger
}

func (d *Config) callbacks(b *beacon.Beacon) {
	for _, fn := range d.beaconCbs {
		fn(b)
	}
}

func (d *Config) applyDkgCallback(share *key.Share) {
	if d.dkgCallback != nil {
		d.dkgCallback(share)
	}
}

// WithGrpcOptions applies grpc dialling option used when a drand node actively
// contacts another.
func WithGrpcOptions(opts ...grpc.DialOption) ConfigOption {
	return func(d *Config) {
		d.grpcOpts = opts
	}
}

// WithCallOption applies grpc options when drand calls a gRPC method.
func WithCallOption(opts ...grpc.CallOption) ConfigOption {
	return func(d *Config) {
		d.callOpts = opts
	}
}

// WithDkgTimeout sets the timeout under which the DKG must finish.
func WithDkgTimeout(t time.Duration) ConfigOption {
	return func(d *Config) {
		d.dkgTimeout = t
	}
}

// WithBoltOptions applies boltdb specific options when storing random beacons.
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

// WithConfigFolder sets the base configuration folder to the given string.
func WithConfigFolder(folder string) ConfigOption {
	return func(d *Config) {
		d.configFolder = folder
		d.dbFolder = path.Join(d.configFolder, DefaultDbFolder)
	}
}

// WithBeaconCallback sets a function that is called each time a new random
// beacon is generated.
func WithBeaconCallback(fn func(*beacon.Beacon)) ConfigOption {
	return func(d *Config) {
		d.beaconCbs = append(d.beaconCbs, fn)
	}
}

// WithDKGCallback sets a function that is called when the DKG finishes. It
// passes in the share of this node and the distributed public key generated.
func WithDKGCallback(fn func(*key.Share)) ConfigOption {
	return func(d *Config) {
		d.dkgCallback = fn
	}
}

// WithInsecure allows drand to listen on standard non-encrypted port and to
// contact other nodes over non-encrypted TCP connections.
func WithInsecure() ConfigOption {
	return func(d *Config) {
		d.insecure = true
	}
}

// WithTLS registers the certificates and private key path so drand can accept
// and issue connections using TLS.
func WithTLS(certPath, keyPath string) ConfigOption {
	return func(d *Config) {
		d.certPath = certPath
		d.keyPath = keyPath
	}
}

// WithTrustedCerts saves the certificates at the given paths and forces drand
// to trust them. Mostly useful for testing.
func WithTrustedCerts(certPaths ...string) ConfigOption {
	return func(d *Config) {
		if d.certmanager == nil {
			d.certmanager = net.NewCertManager()
		}
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

// WithControlPort specifies which port on localhost the ListenerControl should
// bind to.
func WithControlPort(port string) ConfigOption {
	return func(d *Config) {
		d.controlPort = port
	}
}

// WithLogLevel sets the logging verbosity to the given level.
func WithLogLevel(level int) ConfigOption {
	return func(d *Config) {
		d.logger = log.NewLogger(level)
	}
}

func getPeriod(g *key.Group) time.Duration {
	if g.Period == time.Duration(0) {
		return DefaultBeaconPeriod
	}
	return g.Period
}

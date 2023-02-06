package core

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/jmoiron/sqlx"
	clock "github.com/jonboulle/clockwork"
	bolt "go.etcd.io/bbolt"
	"google.golang.org/grpc"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/postgresdb/database"
	"github.com/drand/drand/common"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
)

// ConfigOption is a function that applies a specific setting to a Config.
type ConfigOption func(*Config)

// Config holds all relevant information for a drand node to run.
type Config struct {
	configFolder      string
	version           string
	privateListenAddr string
	publicListenAddr  string
	controlPort       string
	dbStorageEngine   chain.StorageType
	insecure          bool
	dkgTimeout        time.Duration
	grpcOpts          []grpc.DialOption
	callOpts          []grpc.CallOption
	boltOpts          *bolt.Options
	pgDSN             string
	pgConn            *sqlx.DB
	memDBSize         int
	dkgCallback       func(*key.Share, *key.Group)
	certPath          string
	keyPath           string
	certmanager       *net.CertManager
	logger            log.Logger
	clock             clock.Clock
}

// NewConfig returns the config to pass to drand with the default options set
// and the updated values given by the options.
func NewConfig(opts ...ConfigOption) *Config {
	d := &Config{
		configFolder: DefaultConfigFolder(),
		dkgTimeout:   DefaultDKGTimeout,
		controlPort:  DefaultControlPort,
		logger:       log.DefaultLogger(),
		clock:        clock.NewRealClock(),
	}
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

// ConfigFolderMB returns the folder under which multi-beacon drand stores all its
// configuration.
func (d *Config) ConfigFolderMB() string {
	return path.Join(d.configFolder, common.MultiBeaconFolder)
}

// DBFolder returns the folder under which drand stores db file specifically.
// If beacon id is empty, it will use the default value
func (d *Config) DBFolder(beaconID string) string {
	return path.Join(d.ConfigFolderMB(), common.GetCanonicalBeaconID(beaconID), DefaultDBFolder)
}

// Certs returns all custom certs currently being trusted by drand.
func (d *Config) Certs() *net.CertManager {
	return d.certmanager
}

// PrivateListenAddress returns the given default address or the listen address stored
// in the config thanks to WithPrivateListenAddress
func (d *Config) PrivateListenAddress(defaultAddr string) string {
	if d.privateListenAddr != "" {
		return d.privateListenAddr
	}
	return defaultAddr
}

// PublicListenAddress returns the given default address or the listen address stored
// in the config thanks to WithPublicListenAddress
func (d *Config) PublicListenAddress(defaultAddr string) string {
	if d.publicListenAddr != "" {
		return d.publicListenAddr
	}
	return defaultAddr
}

// Version returns the configured version of the binary
func (d *Config) Version() string {
	return d.version
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

func (d *Config) applyDkgCallback(share *key.Share, group *key.Group) {
	if d.dkgCallback != nil {
		d.dkgCallback(share, group)
	}
}

// WithGrpcOptions applies grpc dialing option used when a drand node actively
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

// BoltOptions returns the options given to the bolt db
func (d *Config) BoltOptions() *bolt.Options {
	return d.boltOpts
}

// WithDBStorageEngine allows setting the specific storage type
func WithDBStorageEngine(engine chain.StorageType) ConfigOption {
	return func(d *Config) {
		d.dbStorageEngine = engine
	}
}

// WithPgDSN applies PosgresSQL specific options to the PG store.
// It will also create a new database connection.
func WithPgDSN(dsn string) ConfigOption {
	return func(d *Config) {
		d.pgDSN = dsn

		if d.dbStorageEngine != chain.PostgreSQL {
			// TODO (dlsniper): Would be nice to have a log here. It needs to be injected somehow.
			return
		}

		pgConf, err := database.ConfigFromDSN(dsn)
		if err != nil {
			panic(err)
		}

		//nolint:gomnd // We want a reasonable timeout to connect to the database. If it's not done in 5 seconds, then there are bigger problems.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		d.pgConn, err = database.Open(ctx, pgConf)
		if err != nil {
			err := fmt.Errorf("error while attempting to connect to the database: dsn: %s %w", dsn, err)
			panic(err)
		}
	}
}

// PgDSN returns the PostgreSQL specific DSN configuration.
func (d *Config) PgDSN() string {
	return d.pgDSN
}

func WithMemDBSize(bufferSize int) ConfigOption {
	return func(d *Config) {
		//nolint:gomnd // We want to have a guard here. And it's number 10. It's higher than 1 or 2 to allow for chained mode
		if bufferSize < 10 {
			err := fmt.Errorf("in-memory buffer size cannot be smaller than 10, currently %d, recommended at least 2000", bufferSize)
			panic(err)
		}
		d.memDBSize = bufferSize
	}
}

// WithConfigFolder sets the base configuration folder to the given string.
func WithConfigFolder(folder string) ConfigOption {
	return func(d *Config) {
		d.configFolder = folder
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
		if len(certPaths) < 1 {
			return
		}
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

// WithPublicListenAddress specifies the address the drand instance should bind to. It
// is useful if you want to advertise a public proxy address and the drand
// instance runs behind your network.
func WithPublicListenAddress(addr string) ConfigOption {
	return func(d *Config) {
		d.publicListenAddr = addr
	}
}

// WithPrivateListenAddress specifies the address the drand instance should bind to. It
// is useful if you want to advertise a public proxy address and the drand
// instance runs behind your network.
func WithPrivateListenAddress(addr string) ConfigOption {
	return func(d *Config) {
		d.privateListenAddr = addr
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
func WithLogLevel(level int, jsonFormat bool) ConfigOption {
	return func(d *Config) {
		if jsonFormat {
			d.logger = log.NewJSONLogger(nil, level)
		} else {
			d.logger = log.NewLogger(nil, level)
		}
	}
}

// WithVersion sets a version for drand, a visible string to other peers.
func WithVersion(version string) ConfigOption {
	return func(d *Config) {
		d.version = version
	}
}

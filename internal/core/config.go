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

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/internal/chain"
	"github.com/drand/drand/v2/internal/chain/postgresdb/database"
)

// ConfigOption is a function that applies a specific setting to a Config.
type ConfigOption func(*Config)

// Config holds all relevant information for a drand node to run.
type Config struct {
	configFolder          string
	version               string
	privateListenAddr     string
	publicListenAddr      string
	controlPort           string
	dbStorageEngine       chain.StorageType
	dkgTimeout            time.Duration
	dkgKickoffGracePeriod time.Duration
	dkgPhaseTimeout       time.Duration
	grpcOpts              []grpc.DialOption
	callOpts              []grpc.CallOption
	boltOpts              *bolt.Options
	pgDSN                 string
	pgConn                *sqlx.DB
	memDBSize             int
	dkgCallback           func(context.Context, *key.Group)
	logger                log.Logger
	clock                 clock.Clock
	tracesEndpoint        string
	tracesProbability     float64
}

// NewConfig returns the config to pass to drand with the default options set
// and the updated values given by the options.
func NewConfig(l log.Logger, opts ...ConfigOption) *Config {
	d := &Config{
		configFolder:          DefaultConfigFolder(),
		dkgTimeout:            DefaultDKGPhaseTimeout,
		dkgKickoffGracePeriod: DefaultDKGKickoffGracePeriod,
		dkgPhaseTimeout:       DefaultDKGPhaseTimeout,
		controlPort:           DefaultControlPort,
		logger:                l,
		clock:                 clock.NewRealClock(),
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

func WithDkgKickoffGracePeriod(t time.Duration) ConfigOption {
	return func(d *Config) {
		d.dkgKickoffGracePeriod = t
	}
}

func WithDkgPhaseTimeout(t time.Duration) ConfigOption {
	return func(d *Config) {
		d.dkgPhaseTimeout = t
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

		//nolint:mnd // We want a reasonable timeout to connect to the database. If it's not done in 5 seconds, then there are bigger problems.
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
		//nolint:mnd // We want to have a guard here. And it's number 10. It's higher than 1 or 2 to allow for chained mode
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

func WithNamedLogger(name string) ConfigOption {
	return func(d *Config) {
		d.logger = d.logger.Named(name)
	}
}

// WithVersion sets a version for drand, a visible string to other peers.
func WithVersion(version string) ConfigOption {
	return func(d *Config) {
		d.version = version
	}
}

// WithTracesEndpoint sets the receiver for the tracing data
func WithTracesEndpoint(tracesEndpoint string) ConfigOption {
	return func(d *Config) {
		d.tracesEndpoint = tracesEndpoint
	}
}

// TracesEndpoint retrieves the configured tracing data endpoint
func (d *Config) TracesEndpoint() string {
	return d.tracesEndpoint
}

// WithTracesProbability sets the probability of for traces to be collected and sent to the server
func WithTracesProbability(tracesProbability float64) ConfigOption {
	return func(d *Config) {
		d.tracesProbability = tracesProbability
	}
}

// TracesProbability retrieves the probability of for traces to be collected and sent to the server
func (d *Config) TracesProbability() float64 {
	return d.tracesProbability
}

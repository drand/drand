package core

import (
	"path"
	"time"

	bolt "github.com/coreos/bbolt"
	"github.com/dedis/drand/beacon"
	"github.com/dedis/drand/dkg"
	"google.golang.org/grpc"
)

const defaultConfigFolder = ".drand"
const defaultDbFolder = "db"
const defaultBeaconPeriod time.Duration = 1 * time.Minute

type DrandOptions func(*drandOpts)

type drandOpts struct {
	configFolder string
	dbFolder     string
	grpcOpts     []grpc.DialOption
	dkgTimeout   time.Duration
	boltOpts     *bolt.Options
	beaconPeriod time.Duration
	beaconCbs    []func(*beacon.Beacon)
}

func newDrandOpts(opts ...DrandOptions) *drandOpts {
	d := &drandOpts{
		configFolder: defaultConfigFolder,
		grpcOpts:     []grpc.DialOption{grpc.WithInsecure()},
		dkgTimeout:   dkg.DefaultTimeout,
		dbFolder:     path.Join(defaultConfigFolder, defaultDbFolder),
		beaconPeriod: defaultBeaconPeriod,
	}
	for i := range opts {
		opts[i](d)
	}
	return d
}

func (d *drandOpts) callbacks(b *beacon.Beacon) {
	for _, fn := range d.beaconCbs {
		fn(b)
	}
}

func WithGrpcOptions(opts ...grpc.DialOption) DrandOptions {
	return func(d *drandOpts) {
		d.grpcOpts = opts
	}
}

func WithDkgTimeout(t time.Duration) DrandOptions {
	return func(d *drandOpts) {
		d.dkgTimeout = t
	}
}

func WithBoltOptions(opts *bolt.Options) DrandOptions {
	return func(d *drandOpts) {
		d.boltOpts = opts
	}
}

// WithDbFolder sets the path folder for the db file. This path is NOT relative
// to the DrandFolder path if set.
func WithDbFolder(folder string) DrandOptions {
	return func(d *drandOpts) {
		d.dbFolder = folder
	}
}

func WithConfigFolder(folder string) DrandOptions {
	return func(d *drandOpts) {
		d.configFolder = folder
	}
}

func WithBeaconPeriod(period time.Duration) DrandOptions {
	return func(d *drandOpts) {
		d.beaconPeriod = period
	}
}

func WithBeaconCallback(fn func(*beacon.Beacon)) DrandOptions {
	return func(d *drandOpts) {
		d.beaconCbs = append(d.beaconCbs, fn)
	}
}

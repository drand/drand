package dkg

import (
	"sync"
	"time"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/internal/net"
	"github.com/drand/drand/v2/internal/util"
)

type Process struct {
	lock           sync.Mutex
	store          Store
	internalClient net.DKGClient
	// TODO: remove post v2, as only necessary for upgrade path from v1->v2
	protocolClient   net.ProtocolClient
	beaconIdentifier BeaconIdentifier
	log              log.Logger
	config           Config
	// this is public in order to replace it in the test code to simulate failures
	Executions map[string]Broadcast
	// a set of the packets that have been seen already for easy deduping
	SeenPackets   map[string]bool
	completedDKGs *util.FanOutChan[SharingOutput]
	close         chan struct{}
}

type Config struct {
	// the length of time after which this node will abort a DKG
	Timeout time.Duration

	// the length of time the phaser should use when moving between DKG phases
	TimeBetweenDKGPhases time.Duration

	// the length of time a node should wait before broadcasting DKG packets in the execution phase
	// to allow other nodes to set up their echo broadcast to prevent race conditions
	KickoffGracePeriod time.Duration

	// whether or not to skip verifying the cryptographic material in the DKG... almost certainly should be false
	SkipKeyVerification bool
}

type ExecutionOutput struct {
	FinalGroup *key.Group
	KeyShare   *key.Share
}

type SharingOutput struct {
	BeaconID string
	Old      *DBState
	New      DBState
}

type Store interface {
	// GetCurrent returns the current DKG information, finished DKG information or fresh DKG information,
	// depending on the state of the world
	GetCurrent(beaconID string) (*DBState, error)

	// GetFinished returns the last completed DKG state (i.e. completed or aborted), or nil if one has not been finished
	GetFinished(beaconID string) (*DBState, error)

	// SaveCurrent stores a DKG packet for an ongoing DKG
	SaveCurrent(beaconID string, state *DBState) error

	// SaveFinished stores a completed, successful DKG and overwrites the current packet
	SaveFinished(beaconID string, state *DBState) error

	// Close closes and cleans up any database handles
	Close() error

	// MigrateFromGroupfile takes an existing groupfile and keyshare, and creates a first epoch DKG state for them.
	// It will fail if DKG state already exists for the given beaconID
	// Deprecated: will only exist in 2.0.0 for migration from v1.5.* to 2.0.0
	MigrateFromGroupfile(beaconID string, groupFile *key.Group, share *key.Share) error
}

// BeaconIdentifier is necessary because we need to get our identity on a per-beacon basis from the `DrandDaemon`
// but that would introduce a circular dependency
type BeaconIdentifier interface {
	KeypairFor(beaconID string) (*key.Pair, error)
}

func NewDKGProcess(
	store Store,
	beaconIdentifier BeaconIdentifier,
	completedDKGs *util.FanOutChan[SharingOutput],
	dkgClient net.DKGClient,
	protocolClient net.ProtocolClient,
	config Config,
	l log.Logger,
) *Process {
	return &Process{
		store:            store,
		beaconIdentifier: beaconIdentifier,
		internalClient:   dkgClient,
		protocolClient:   protocolClient,
		log:              l,
		Executions:       make(map[string]Broadcast),
		SeenPackets:      make(map[string]bool),
		config:           config,
		completedDKGs:    completedDKGs,
		close:            make(chan struct{}, 1),
	}
}

func (d *Process) Close() {
	d.lock.Lock()
	defer d.lock.Unlock()
	for _, e := range d.Executions {
		e.Stop()
	}
	close(d.close)
	err := d.store.Close()
	if err != nil {
		d.log.Errorw("error closing the database", "err", err)
	}
	d.completedDKGs.Close()
}

// Migrate takes an existing groupfile and keyshare, and creates a first epoch DKG state for them.
// It will fail if DKG state already exists for the given beaconID
// Deprecated: will only exist in 2.0.0 for migration from v1.5.* to 2.0.0
func (d *Process) Migrate(beaconID string, groupfile *key.Group, share *key.Share) error {
	d.log.Infow("Migrating DKG from group file...", "beaconID", beaconID)

	if err := d.store.MigrateFromGroupfile(beaconID, groupfile, share); err != nil {
		return err
	}

	d.log.Debugw("Completed migration from group file")
	return nil
}

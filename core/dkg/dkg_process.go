package dkg

import (
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
)

//nolint:revive
type DKGProcess struct {
	store            Store
	network          Network
	beaconIdentifier BeaconIdentifier
	log              log.Logger
	config           Config
	// this is public in order to replace it in the test code to simulate failures
	Executions    map[string]Broadcast
	completedDKGs chan<- SharingOutput
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
}

type Network interface {
	Send(
		from *drand.Participant,
		to []*drand.Participant,
		action func(client drand.DKGClient) (*drand.EmptyResponse, error),
	) error
	SendIgnoringConnectionError(
		from *drand.Participant,
		to []*drand.Participant,
		action func(client drand.DKGClient) (*drand.EmptyResponse, error),
	) error
}

// BeaconIdentifier is necessary because we need to get our identity on a per-beacon basis from the `DrandDaemon`
// but that would introduce a circular dependency
type BeaconIdentifier interface {
	KeypairFor(beaconID string) (*key.Pair, error)
}

func NewDKGProcess(
	store Store,
	beaconIdentifier BeaconIdentifier,
	completedDKGs chan<- SharingOutput,
	config Config,
	l log.Logger,
) *DKGProcess {
	return &DKGProcess{
		store:            store,
		network:          &GrpcNetwork{},
		beaconIdentifier: beaconIdentifier,
		log:              l,
		Executions:       make(map[string]Broadcast),
		config:           config,
		completedDKGs:    completedDKGs,
	}
}

func (d *DKGProcess) Close() {
	for _, e := range d.Executions {
		e.Stop()
	}
}

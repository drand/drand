package dkg

import (
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
)

type DKGProcess struct {
	beaconIdentifier BeaconIdentifier
	network          Network
	store            DKGStore
	log              log.Logger
}

type DKGStore interface {
	// GetCurrent returns the current DKG information, finished DKG information or fresh DKG information,
	// depending on the state of the world
	GetCurrent(beaconID string) (*DKGState, error)

	// GetFinished returns the last completed DKG state (i.e. completed or aborted), or nil if one has not been finished
	GetFinished(beaconID string) (*DKGState, error)

	// SaveCurrent stores a DKG packet for an ongoing DKG
	SaveCurrent(beaconID string, state *DKGState) error

	// SaveFinished stores a completed, successful DKG and overwrites the current packet
	SaveFinished(beaconID string, state *DKGState) error

	// Close closes and cleans up any database handles
	Close() error
}

type Network interface {
	Send(from *drand.Participant, to []*drand.Participant, action func(client drand.DKGClient) (*drand.GenericResponseMessage, error)) error
}

// BeaconIdentifier is necessary because we need to get our identity on a per-beacon basis from the `DrandDaemon`
// but that would introduce a circular dependency
type BeaconIdentifier interface {
	IdentityFor(beaconID string) (*key.Identity, error)
}

func NewDKGProcess(store *DKGStore, daemon BeaconIdentifier) *DKGProcess {
	return &DKGProcess{
		store:            *store,
		log:              log.NewLogger(nil, log.LogDebug),
		network:          &GrpcNetwork{},
		beaconIdentifier: daemon,
	}
}

package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/drand/drand/protobuf/drand"
)

func (dd *DrandDaemon) DKGStatus(ctx context.Context, request *drand.DKGStatusRequest) (*drand.DKGStatusResponse, error) {
	beaconID := request.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.DKGStatus(ctx, request)
}

func (dd *DrandDaemon) Command(ctx context.Context, command *drand.DKGCommand) (*drand.EmptyDKGResponse, error) {
	if command.Metadata == nil {
		return nil, errors.New("could not find command metadata to read beaconID")
	}
	beaconID := command.Metadata.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.Command(ctx, command)
}

func (dd *DrandDaemon) Packet(ctx context.Context, packet *drand.GossipPacket) (*drand.EmptyDKGResponse, error) {
	if packet.Metadata == nil {
		return nil, errors.New("could not find command metadata to read beaconID")
	}
	beaconID := packet.Metadata.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.Packet(ctx, packet)
}

func (dd *DrandDaemon) BroadcastDKG(ctx context.Context, packet *drand.DKGPacket) (*drand.EmptyDKGResponse, error) {
	beaconID := packet.Dkg.Metadata.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.BroadcastDKG(ctx, packet)
}

func (dd *DrandDaemon) beaconExists(beaconID string) bool {
	_, exists := dd.beaconProcesses[beaconID]
	return exists
}

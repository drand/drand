package dkg

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/drand/drand/v2/common/tracer"
	"github.com/drand/drand/v2/internal/metrics"

	"github.com/drand/drand/v2/internal/util"
	drand "github.com/drand/drand/v2/protobuf/dkg"
)

// actions_passive contains all internal messaging between nodes triggered by the protocol - things it does automatically
// upon receiving messages from other nodes: storing proposals, aborting when the leader aborts, etc

const ShortSigLength = 8

func (d *Process) Packet(ctx context.Context, packet *drand.GossipPacket) (*drand.EmptyDKGResponse, error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	if packet == nil {
		return nil, errors.New("packet cannot be nil")
	}
	// if there's no metadata on the packet, we won't be able to verify the signature or perform other state changes
	if packet.Metadata == nil {
		return nil, errors.New("packet missing metadata")
	}

	packetName := packetName(packet)
	packetSig := hex.EncodeToString(packet.Metadata.Signature)
	if len(packetSig) < ShortSigLength {
		return nil, errors.New("packet signature is too short")
	}
	shortSig := packetSig[0:ShortSigLength]
	d.log.Debugw("processing DKG gossip packet", "type", packetName, "sig", shortSig)
	_, span := tracer.NewSpan(ctx, fmt.Sprintf("packet.%s", packetName))
	defer span.End()

	// we ignore duplicate packets, so we don't try and store/gossip them ad infinitum
	if d.SeenPackets[packetSig] {
		d.log.Debugw("ignoring duplicate packet", "sig", shortSig)
		return &drand.EmptyDKGResponse{}, nil
	}

	// if we're in the DKG protocol phase, we automatically broadcast it
	if packet.GetDkg() != nil {
		return d.BroadcastDKG(ctx, packet.GetDkg())
	}

	beaconID := packet.Metadata.BeaconID
	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return nil, err
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return nil, err
	}

	// if we have aborted or timed out, we actually want to apply the proposal to the last successful state
	if util.Cont(terminalStates, current.State) {
		finished, err := d.store.GetFinished(beaconID)
		if err != nil {
			return nil, err
		}

		if finished == nil {
			current = NewFreshState(beaconID)
		} else {
			current = finished
		}
	}

	nextState, err := current.Apply(me, packet)
	if err != nil {
		return nil, err
	}

	// we must verify the message against the next state, as the current state upon first proposal will be empty
	err = d.verifyMessage(packet, packet.Metadata, termsFromState(nextState))
	if err != nil {
		return nil, fmt.Errorf("invalid packet signature from %s: %w", packet.Metadata.Address, err)
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	if err != nil {
		return nil, err
	}

	metrics.DKGStateChange(nextState.BeaconID, nextState.Epoch, nextState.Leader == me, uint32(nextState.State))
	recipients := util.Concat(nextState.Joining, nextState.Remaining, nextState.Leaving)
	// we ignore the errors here because it's a best effort gossip
	// however we can continue with execution
	_, _ = d.gossip(me, recipients, packet)
	// we could theoretically ignore when the gossip ends, but due to the mutex we're holding it _could_ lead to a race
	// condition with future requests

	if packet.GetExecute() != nil {
		if err := d.executeDKG(ctx, beaconID, packet.GetExecute().Time.AsTime()); err != nil {
			return nil, err
		}
	}

	return &drand.EmptyDKGResponse{}, nil
}

func commandType(command *drand.DKGCommand) string {
	switch command.Command.(type) {
	case *drand.DKGCommand_Initial:
		return "Initial DKG"
	case *drand.DKGCommand_Resharing:
		return "Resharing"
	case *drand.DKGCommand_Accept:
		return "Accepting"
	case *drand.DKGCommand_Reject:
		return "Rejecting"
	case *drand.DKGCommand_Join:
		return "Joining"
	case *drand.DKGCommand_Execute:

		return "Executing"
	case *drand.DKGCommand_Abort:
		return "Aborting"
	default:
		return "UnknownCommand"
	}
}

func packetName(packet *drand.GossipPacket) string {
	switch packet.Packet.(type) {
	case *drand.GossipPacket_Proposal:
		return "Proposal"
	case *drand.GossipPacket_Accept:
		return "Accept"
	case *drand.GossipPacket_Reject:
		return "Reject"
	case *drand.GossipPacket_Abort:
		return "Abort"
	case *drand.GossipPacket_Execute:
		return "Execute"
	case *drand.GossipPacket_Dkg:
		return "DKG"
	default:
		return "Unknown"
	}
}

// BroadcastDKG gossips internal DKG protocol messages to other nodes (i.e. any messages encapsulated in the Kyber DKG)
func (d *Process) BroadcastDKG(ctx context.Context, packet *drand.DKGPacket) (*drand.EmptyDKGResponse, error) {
	_, span := tracer.NewSpan(ctx, "dkg.BroadcastDKG")
	defer span.End()

	beaconID := packet.Dkg.Metadata.BeaconID
	d.lock.Lock()
	broadcaster := d.Executions[beaconID]
	d.lock.Unlock()
	if broadcaster == nil {
		return nil, errors.New("could not broadcast a DKG message - there may not be a DKG in progress and in the execution phase")
	}

	err := broadcaster.BroadcastDKG(ctx, packet)
	if err != nil {
		return nil, err
	}
	return &drand.EmptyDKGResponse{}, nil
}

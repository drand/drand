package dkg

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/drand/drand/common/key"
	"github.com/drand/drand/internal/metrics"
	"github.com/drand/drand/internal/util"
	"github.com/drand/drand/protobuf/drand"
)

// actions_active contains all the DKG actions that require user interaction: creating a network,
// accepting or rejecting a DKG, getting the status, etc. Both leader and follower interactions are contained herein.

//nolint:gocyclo //
func (d *Process) Command(ctx context.Context, command *drand.DKGCommand) (*drand.EmptyResponse, error) {
	beaconID := command.Metadata.BeaconID
	commandName := commandType(command)
	d.lock.Lock()
	defer d.lock.Unlock()

	// fetch our keypair from the BeaconProcess and remap it into a `Participant`
	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return nil, err
	}

	// pull the latest DKG state from the database
	currentState, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return nil, err
	}

	d.log.Infow("running DKG command", "command", commandName, "beaconID", beaconID)
	// map the current state to the latest state, depending on the type of packet
	var packetToGossip *drand.GossipPacket
	var afterState *DBState
	switch c := command.Command.(type) {
	case *drand.DKGCommand_Initial:
		afterState, packetToGossip, err = d.StartNetwork(ctx, beaconID, me, currentState, c.Initial)
	case *drand.DKGCommand_Resharing:
		afterState, packetToGossip, err = d.StartProposal(ctx, beaconID, me, currentState, c.Resharing)
	case *drand.DKGCommand_Join:
		afterState, packetToGossip, err = d.StartJoin(ctx, beaconID, me, currentState, c.Join)
	case *drand.DKGCommand_Accept:
		afterState, packetToGossip, err = d.StartAccept(ctx, beaconID, me, currentState, c.Accept)
	case *drand.DKGCommand_Reject:
		afterState, packetToGossip, err = d.StartReject(ctx, beaconID, me, currentState, c.Reject)
	case *drand.DKGCommand_Execute:
		afterState, packetToGossip, err = d.StartExecute(ctx, beaconID, me, currentState, c.Execute)
	case *drand.DKGCommand_Abort:
		afterState, packetToGossip, err = d.StartAbort(ctx, beaconID, me, currentState, c.Abort)
	default:
		return nil, errors.New("unrecognized DKG command")
	}

	if err != nil {
		return nil, fmt.Errorf("error running command %s: %w", commandName, err)
	}

	// if there is an output packet to gossip (i.e. if the user isn't joining)
	// then we sign the packet and gossip it to the network
	if packetToGossip != nil {
		recipients := util.Concat(afterState.Joining, afterState.Leaving, afterState.Remaining)
		done, errs := d.gossip(me, recipients, packetToGossip, termsFromState(afterState))
		// if it's a proposal, let's block until it finishes or a timeout
		if command.GetInitial() != nil || command.GetResharing() != nil {
			select {
			case err = <-errs:
				return nil, err
			case <-done:
				break
			}
		}
		// we also add it to the SeenPackets set,
		// so we don't try and reprocess it when it gets gossiped back to us!
		packetSig := hex.EncodeToString(packetToGossip.Metadata.Signature)
		d.SeenPackets[packetSig] = true
	}

	return &drand.EmptyResponse{}, nil
}

func (d *Process) StartNetwork(
	ctx context.Context,
	beaconID string,
	me *drand.Participant,
	state *DBState,
	options *drand.FirstProposalOptions,
) (*DBState, *drand.GossipPacket, error) {
	_, span := metrics.NewSpan(ctx, "dkg.StartNetwork")
	defer span.End()

	genesisTime := options.GenesisTime
	if genesisTime == nil {
		genesisTime = timestamppb.New(time.Now())
	}

	// remap the CLI payload into one useful for applying to the DKG state
	terms := drand.ProposalTerms{
		BeaconID:    options.BeaconID,
		Threshold:   options.Threshold,
		Epoch:       1,
		Timeout:     options.Timeout,
		Leader:      me,
		SchemeID:    options.Scheme,
		GenesisTime: genesisTime,
		// GenesisSeed is created after the DKG, so it cannot exist yet
		GenesisSeed: nil,
		// for the initial proposal, we want the transition time should be the same as the genesis time
		TransitionTime:       genesisTime,
		CatchupPeriodSeconds: options.CatchupPeriodSeconds,
		BeaconPeriodSeconds:  options.PeriodSeconds,
		Joining:              options.Joining,
	}

	// apply our enriched DKG payload onto the current DKG state to create a new state
	nextState, err := state.Proposing(me, &terms)
	if err != nil {
		return nil, nil, err
	}
	if err := d.store.SaveCurrent(beaconID, nextState); err != nil {
		return nil, nil, err
	}

	return nextState, &drand.GossipPacket{
		Packet: &drand.GossipPacket_Proposal{
			Proposal: &terms,
		},
	}, nil
}

func (d *Process) StartProposal(
	ctx context.Context,
	beaconID string,
	me *drand.Participant,
	currentState *DBState,
	options *drand.ProposalOptions,
) (*DBState, *drand.GossipPacket, error) {
	_, span := metrics.NewSpan(ctx, "dkg.StartProposal")
	defer span.End()

	var newEpoch uint32
	if currentState.State == Aborted || currentState.State == TimedOut {
		newEpoch = currentState.Epoch
	} else {
		newEpoch = currentState.Epoch + 1
	}

	terms := drand.ProposalTerms{
		BeaconID:             beaconID,
		Threshold:            options.Threshold,
		Epoch:                newEpoch,
		SchemeID:             currentState.SchemeID,
		BeaconPeriodSeconds:  uint32(currentState.BeaconPeriod.Seconds()),
		CatchupPeriodSeconds: options.CatchupPeriodSeconds,
		GenesisTime:          timestamppb.New(currentState.GenesisTime),
		GenesisSeed:          currentState.GenesisSeed,
		TransitionTime:       options.TransitionTime,
		Timeout:              options.Timeout,
		Leader:               me,
		Joining:              options.Joining,
		Remaining:            options.Remaining,
		Leaving:              options.Leaving,
	}

	nextState, err := currentState.Proposing(me, &terms)
	if err != nil {
		return nil, nil, err
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	if err != nil {
		return nil, nil, err
	}

	return nextState,
		&drand.GossipPacket{
			Packet: &drand.GossipPacket_Proposal{
				Proposal: &terms,
			},
		},
		nil
}

func (d *Process) StartAbort(
	ctx context.Context,
	beaconID string,
	me *drand.Participant,
	current *DBState,
	_ *drand.AbortOptions,
) (*DBState, *drand.GossipPacket, error) {
	_, span := metrics.NewSpan(ctx, "dkg.StartAbort")
	defer span.End()

	nextState, err := current.StartAbort(me)
	if err != nil {
		return nil, nil, err
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	if err != nil {
		return nil, nil, err
	}

	return nextState, &drand.GossipPacket{
		Packet: &drand.GossipPacket_Abort{
			Abort: &drand.AbortDKG{Reason: "none"},
		},
	}, nil
}

func (d *Process) StartExecute(
	ctx context.Context,
	beaconID string,
	me *drand.Participant,
	state *DBState,
	_ *drand.ExecutionOptions,
) (*DBState, *drand.GossipPacket, error) {
	_, span := metrics.NewSpan(ctx, "dkg.StartExecute")
	defer span.End()

	nextState, err := state.StartExecuting(me)
	if err != nil {
		return nil, nil, err
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	if err != nil {
		return nil, nil, err
	}

	err = d.executeDKG(ctx, beaconID)
	if err != nil {
		return nil, nil, err
	}

	return nextState, &drand.GossipPacket{
		Packet: &drand.GossipPacket_Execute{
			Execute: &drand.StartExecution{},
		},
	}, nil
}

func (d *Process) StartJoin(
	ctx context.Context,
	beaconID string,
	me *drand.Participant,
	state *DBState,
	options *drand.JoinOptions,
) (*DBState, *drand.GossipPacket, error) {
	_, span := metrics.NewSpan(ctx, "dkg.StartJoin")
	defer span.End()

	var previousGroupFile *key.Group
	if state.Epoch > 1 {
		if options.GroupFile == nil {
			return nil, nil, errors.New("group file required to join after the first epoch")
		}
		p, err := util.ParseGroupFileBytes(options.GroupFile)
		if err != nil {
			return nil, nil, err
		}
		previousGroupFile = p
	}

	nextState, err := state.Joined(me, previousGroupFile)
	if err != nil {
		return nil, nil, err
	}

	// joiners don't need to gossip anything
	return nextState, nil, d.store.SaveCurrent(beaconID, nextState)
}

//nolint:dupl // it's similar but not the same
func (d *Process) StartAccept(
	ctx context.Context,
	beaconID string,
	me *drand.Participant,
	state *DBState,
	_ *drand.AcceptOptions,
) (*DBState, *drand.GossipPacket, error) {
	_, span := metrics.NewSpan(ctx, "dkg.StartAccept")
	defer span.End()

	nextState, err := state.Accepted(me)
	if err != nil {
		return nil, nil, err
	}
	err = d.store.SaveCurrent(beaconID, nextState)
	if err != nil {
		return nil, nil, err
	}

	return nextState, &drand.GossipPacket{
		Packet: &drand.GossipPacket_Accept{
			Accept: &drand.AcceptProposal{
				Acceptor: me,
			},
		},
	}, nil
}

//nolint:dupl // it's similar but not the same
func (d *Process) StartReject(
	ctx context.Context,
	beaconID string,
	me *drand.Participant,
	state *DBState,
	_ *drand.RejectOptions,
) (*DBState, *drand.GossipPacket, error) {
	_, span := metrics.NewSpan(ctx, "dkg.StartReject")
	defer span.End()

	nextState, err := state.Rejected(me)
	if err != nil {
		return nil, nil, err
	}
	err = d.store.SaveCurrent(beaconID, nextState)
	if err != nil {
		return nil, nil, err
	}

	return nextState, &drand.GossipPacket{
		Packet: &drand.GossipPacket_Reject{
			Reject: &drand.RejectProposal{
				Rejector: me,
			},
		},
	}, nil
}

func (d *Process) DKGStatus(ctx context.Context, request *drand.DKGStatusRequest) (*drand.DKGStatusResponse, error) {
	_, span := metrics.NewSpan(ctx, "dkg.Status")
	defer span.End()

	finished, err := d.store.GetFinished(request.BeaconID)
	if err != nil {
		return nil, err
	}

	current, err := d.store.GetCurrent(request.BeaconID)
	if err != nil {
		return nil, err
	}
	var finalGroup []string
	if current.FinalGroup != nil {
		finalGroup := make([]string, len(current.FinalGroup.Nodes))
		for i, v := range current.FinalGroup.Nodes {
			finalGroup[i] = v.Addr
		}
	}
	currentEntry := drand.DKGEntry{
		BeaconID:       current.BeaconID,
		State:          uint32(current.State),
		Epoch:          current.Epoch,
		Threshold:      current.Threshold,
		Timeout:        timestamppb.New(current.Timeout),
		GenesisTime:    timestamppb.New(current.GenesisTime),
		GenesisSeed:    current.GenesisSeed,
		TransitionTime: timestamppb.New(current.TransitionTime),
		Leader:         current.Leader,
		Remaining:      current.Remaining,
		Joining:        current.Joining,
		Leaving:        current.Leaving,
		Acceptors:      current.Acceptors,
		Rejectors:      current.Rejectors,
		FinalGroup:     finalGroup,
	}

	if finished == nil {
		return &drand.DKGStatusResponse{
			Current: &currentEntry,
		}, nil
	}
	finishedFinalGroup := make([]string, len(finished.FinalGroup.Nodes))
	for i, v := range finished.FinalGroup.Nodes {
		finishedFinalGroup[i] = v.Addr
	}

	return &drand.DKGStatusResponse{
		Complete: &drand.DKGEntry{
			BeaconID:       finished.BeaconID,
			State:          uint32(finished.State),
			Epoch:          finished.Epoch,
			Threshold:      finished.Threshold,
			Timeout:        timestamppb.New(finished.Timeout),
			GenesisTime:    timestamppb.New(finished.GenesisTime),
			GenesisSeed:    finished.GenesisSeed,
			TransitionTime: timestamppb.New(finished.TransitionTime),
			Leader:         finished.Leader,
			Remaining:      finished.Remaining,
			Joining:        finished.Joining,
			Leaving:        finished.Leaving,
			Acceptors:      finished.Acceptors,
			Rejectors:      finished.Rejectors,
			FinalGroup:     finishedFinalGroup,
		},
		Current: &currentEntry,
	}, nil
}

// identityForBeacon grabs the key.Pair from a BeaconProcess and marshals it to a drand.Participant
func (d *Process) identityForBeacon(beaconID string) (*drand.Participant, error) {
	identity, err := d.beaconIdentifier.KeypairFor(beaconID)
	if err != nil {
		return nil, err
	}

	return util.PublicKeyAsParticipant(identity.Public)
}

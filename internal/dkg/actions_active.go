package dkg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/metrics"
	"github.com/drand/drand/v2/internal/net"
	drand "github.com/drand/drand/v2/protobuf/dkg"
	proto "github.com/drand/drand/v2/protobuf/drand"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/tracer"
	"github.com/drand/drand/v2/internal/util"
)

// actions_active contains all the DKG actions that require user interaction: creating a network,
// accepting or rejecting a DKG, getting the status, etc. Both leader and follower interactions are contained herein.

//nolint:gocyclo,funlen
func (d *Process) Command(ctx context.Context, command *drand.DKGCommand) (*drand.EmptyDKGResponse, error) {
	if command == nil {
		return nil, errors.New("command cannot be nil")
	}
	if command.Metadata == nil {
		return nil, errors.New("command metadata cannot be nil")
	}
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

	// if we have aborted or timed out, we actually want to apply the proposal to the last successful state
	if util.Cont(terminalStates, currentState.State) {
		finalState, err := d.store.GetFinished(beaconID)
		if err != nil {
			return nil, err
		}
		if finalState == nil {
			currentState = NewFreshState(beaconID)
		} else {
			currentState = finalState
		}
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
		// packetToGossip will be always be nil for StartJoin
		afterState, packetToGossip, err = d.StartJoin(ctx, beaconID, me, currentState, c.Join)
	case *drand.DKGCommand_Accept:
		afterState, packetToGossip, err = d.StartAccept(ctx, beaconID, me, currentState, c.Accept)
	case *drand.DKGCommand_Reject:
		afterState, packetToGossip, err = d.StartReject(ctx, beaconID, me, currentState, c.Reject)
	case *drand.DKGCommand_Execute:
		afterState, packetToGossip, err = d.StartExecute(ctx, beaconID, me, currentState, c.Execute)
	case *drand.DKGCommand_Abort:
		afterState, packetToGossip, err = d.StartAbort(ctx, beaconID, currentState, c.Abort)
	default:
		return nil, errors.New("unrecognized DKG command")
	}

	if err != nil {
		return nil, fmt.Errorf("error running command %s: %w", commandName, err)
	}

	metrics.DKGStateChange(afterState.BeaconID, afterState.Epoch, afterState.Leader == me, uint32(afterState.State))
	// if there is an output packet to gossip (i.e. if the user isn't joining)
	// then we sign the packet and gossip it to the network
	if packetToGossip != nil {
		// then we gossip to the joiners and remainers
		recipients := util.Concat(afterState.Joining, afterState.Remaining)
		terms := termsFromState(afterState)
		metadata, err := d.signMessage(beaconID, packetToGossip, terms)
		if err != nil {
			return nil, err
		}

		packetToGossip.Metadata = metadata
		errs := d.gossip(me, recipients, packetToGossip)
		// we gossip the leavers separately - if it fails, no big deal
		_ = d.gossip(me, afterState.Leaving, packetToGossip)

		// if it's a proposal, let's block until it finishes gossiping or a timeout,
		// because we want to be sure everybody received it
		// QUESTION: do we _really_ want to fail on errors? we will probably have to abort if that's the case
		if command.GetInitial() != nil || command.GetResharing() != nil {
			allErrs := make([]error, 0, len(recipients))
			// for will block on errs until errs is closed
			for e := range errs {
				allErrs = append(allErrs, e)
			}
			if len(allErrs) > 0 {
				return nil, errors.Join(allErrs...)
			}
		}
	}

	return &drand.EmptyDKGResponse{}, nil
}

func (d *Process) StartNetwork(
	ctx context.Context,
	beaconID string,
	me *drand.Participant,
	state *DBState,
	options *drand.FirstProposalOptions,
) (*DBState, *drand.GossipPacket, error) {
	_, span := tracer.NewSpan(ctx, "dkg.StartNetwork")
	defer span.End()

	genesisTime := options.GenesisTime
	if genesisTime == nil {
		genesisTime = timestamppb.New(time.Now())
	}

	// remap the CLI payload into one useful for applying to the DKG state
	terms := drand.ProposalTerms{
		BeaconID:    beaconID,
		Threshold:   options.Threshold,
		Epoch:       1,
		Timeout:     options.Timeout,
		Leader:      me,
		SchemeID:    options.Scheme,
		GenesisTime: genesisTime,
		// GenesisSeed is created after the DKG, so it cannot exist yet
		GenesisSeed:          nil,
		CatchupPeriodSeconds: options.CatchupPeriodSeconds,
		BeaconPeriodSeconds:  options.PeriodSeconds,
		Joining:              util.Filter(options.Joining, util.NonEmpty),
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
		// this gets set in the enclosing scope
		Metadata: nil,
	}, nil
}

func asIdentity(response *proto.IdentityResponse) (key.Identity, error) {
	sch, err := crypto.GetSchemeByID(response.GetSchemeName())
	if err != nil {
		return key.Identity{}, fmt.Errorf("peer return key of scheme %s, which was not found", response.GetSchemeName())
	}

	pk := sch.KeyGroup.Point()
	err = pk.UnmarshalBinary(response.Key)
	if err != nil {
		return key.Identity{}, err
	}
	return key.Identity{
		Key:       pk,
		Addr:      response.Address,
		Signature: response.Signature,
		Scheme:    sch,
	}, nil
}

//nolint:funlen
func (d *Process) StartProposal(
	ctx context.Context,
	beaconID string,
	me *drand.Participant,
	currentState *DBState,
	options *drand.ProposalOptions,
) (*DBState, *drand.GossipPacket, error) {
	_, span := tracer.NewSpan(ctx, "dkg.StartProposal")
	defer span.End()

	// Migration path for v1.5.8 -> v2 upgrade
	// TODO: remove it in the future
	if currentState.Epoch == 1 && currentState.State == Complete {
		d.log.Info("First proposal after upgrade to v2 - migrating keys from group file")

		// migration from v1 -> v2 makes all parties in the v1 group file 'joiners'
		// the DKGProcess migration path will set old signatures to `nil`, hence we check it here
	LOOP:
		for i, j := range currentState.Joining {
			// leavers may not be online, so we don't want to block the proposal by trying to get their new keys
			for _, l := range options.Leaving {
				if l.Address == j.Address {
					d.log.Debugw("skipping key migration for leaver", "address", j.Address)
					continue LOOP
				}
			}
			// the migration path sets the signature in the database to nil; if we have a signature, we can assume that
			// the reshare is in fact a new network, and not migrated from v1
			if j.Signature != nil {
				d.log.Debugw("proposal migration - signature not nil, skipping", "sig len", len(j.Signature))
				continue
			}
			// fetch their public key via gRPC
			response, err := d.protocolClient.GetIdentity(ctx, net.CreatePeer(j.Address), &proto.IdentityRequest{Metadata: &proto.Metadata{
				BeaconID:  beaconID,
				ChainHash: nil,
			}})
			if err != nil {
				return nil, nil, err
			}

			// verify its signature
			identity, err := asIdentity(response)
			if err != nil {
				return nil, nil, err
			}

			err = identity.ValidSignature()
			if err != nil {
				return nil, nil, err
			}

			// update the signature mapping
			updatedParticipant := drand.Participant{
				Address:   j.Address,
				Key:       response.Key,
				Signature: response.Signature,
			}

			currentState.Joining[i] = &updatedParticipant
			// if they were the last leader, update their key also
			if currentState.Leader.Address == j.Address {
				currentState.Leader = &updatedParticipant
			}

			// store it in the DB
			err = d.store.SaveCurrent(beaconID, currentState)
			if err != nil {
				return nil, nil, err
			}
			err = d.store.SaveFinished(beaconID, currentState)
			if err != nil {
				return nil, nil, err
			}
			d.log.Info("Key migration complete")
		}
	}

	terms := drand.ProposalTerms{
		BeaconID:             beaconID,
		Threshold:            options.Threshold,
		Epoch:                currentState.Epoch + 1,
		SchemeID:             currentState.SchemeID,
		BeaconPeriodSeconds:  uint32(currentState.BeaconPeriod.Seconds()),
		CatchupPeriodSeconds: options.CatchupPeriodSeconds,
		GenesisTime:          timestamppb.New(currentState.GenesisTime),
		GenesisSeed:          currentState.GenesisSeed,
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
	current *DBState,
	_ *drand.AbortOptions,
) (*DBState, *drand.GossipPacket, error) {
	_, span := tracer.NewSpan(ctx, "dkg.StartAbort")
	defer span.End()

	nextState, err := current.StartAbort()
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
	_, span := tracer.NewSpan(ctx, "dkg.StartExecute")
	defer span.End()

	nextState, err := state.StartExecuting(me)
	if err != nil {
		return nil, nil, err
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	if err != nil {
		return nil, nil, err
	}

	kickoffTime := time.Now().Add(d.config.KickoffGracePeriod)
	err = d.executeDKG(ctx, beaconID, kickoffTime)
	if err != nil {
		return nil, nil, err
	}

	return nextState, &drand.GossipPacket{
		Packet: &drand.GossipPacket_Execute{
			Execute: &drand.StartExecution{
				Time: timestamppb.New(kickoffTime),
			},
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
	_, span := tracer.NewSpan(ctx, "dkg.StartJoin")
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
	_, span := tracer.NewSpan(ctx, "dkg.StartAccept")
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
	_, span := tracer.NewSpan(ctx, "dkg.StartReject")
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
	_, span := tracer.NewSpan(ctx, "dkg.Status")
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
		BeaconID:    current.BeaconID,
		State:       uint32(current.State),
		Epoch:       current.Epoch,
		Threshold:   current.Threshold,
		Timeout:     timestamppb.New(current.Timeout),
		GenesisTime: timestamppb.New(current.GenesisTime),
		GenesisSeed: current.GenesisSeed,
		Leader:      current.Leader,
		Remaining:   current.Remaining,
		Joining:     current.Joining,
		Leaving:     current.Leaving,
		Acceptors:   current.Acceptors,
		Rejectors:   current.Rejectors,
		FinalGroup:  finalGroup,
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
			BeaconID:    finished.BeaconID,
			State:       uint32(finished.State),
			Epoch:       finished.Epoch,
			Threshold:   finished.Threshold,
			Timeout:     timestamppb.New(finished.Timeout),
			GenesisTime: timestamppb.New(finished.GenesisTime),
			GenesisSeed: finished.GenesisSeed,
			Leader:      finished.Leader,
			Remaining:   finished.Remaining,
			Joining:     finished.Joining,
			Leaving:     finished.Leaving,
			Acceptors:   finished.Acceptors,
			Rejectors:   finished.Rejectors,
			FinalGroup:  finishedFinalGroup,
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

package dkg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/drand/drand/common"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/metrics"
	"github.com/drand/drand/internal/util"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/sign/schnorr"
)

func (d *Process) executeDKG(ctx context.Context, beaconID string, executionStartTime time.Time) error {
	// set up the DKG broadcaster for first so we're ready to broadcast DKG messages
	dkgConfig, err := d.setupDKG(ctx, beaconID)
	if err != nil {
		return err
	}

	d.log.Infow("DKG execution setup successful", "beaconID", beaconID)

	go func(config dkg.Config) {
		// wait until the time set by the leader for kicking off the DKG to allow other nodes to get
		// the requisite packets
		time.Sleep(time.Until(executionStartTime))
		err := d.executeAndFinishDKG(ctx, beaconID, config)
		if err != nil {
			d.log.Errorw("there was an error during the DKG!", "beaconID", beaconID, "error", err)
		}
	}(*dkgConfig)
	return nil
}

func (d *Process) setupDKG(ctx context.Context, beaconID string) (*dkg.Config, error) {
	ctx, span := metrics.NewSpan(ctx, "dkg.setupDKG")
	defer span.End()
	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return nil, err
	}

	lastCompleted, err := d.store.GetFinished(beaconID)
	if err != nil {
		return nil, err
	}

	keypair, err := d.beaconIdentifier.KeypairFor(beaconID)
	if err != nil {
		return nil, err
	}
	me, err := util.PublicKeyAsParticipant(keypair.Public)
	if err != nil {
		return nil, err
	}

	sortedParticipants := util.SortedByPublicKey(append(current.Remaining, current.Joining...))
	var config *dkg.Config
	if lastCompleted == nil {
		config, err = d.initialDKGConfig(current, keypair, sortedParticipants)
	} else {
		config, err = d.reshareDKGConfig(current, lastCompleted, keypair, sortedParticipants)
	}
	if err != nil {
		return nil, err
	}

	// create the network over which to send all the DKG packets
	board, err := newEchoBroadcast(
		ctx,
		d.internalClient,
		d.log,
		common.GetAppVersion(),
		beaconID,
		me.Address,
		sortedParticipants,
		keypair.Scheme(),
		config,
	)
	if err != nil {
		return nil, err
	}

	// we need some state on the DKG process in order to process any incoming gossip messages from the DKG
	// if other nodes try to send us DKG messages before this is set we're in trouble
	d.Executions[beaconID] = board

	return config, nil
}

// this is done rarely and is a shared object: no good reason not to use a clone (and it makes the race checker happy)
//
//nolint:funlen
func (d *Process) executeAndFinishDKG(ctx context.Context, beaconID string, config dkg.Config) error {
	ctx, span := metrics.NewSpan(ctx, "dkg.executeAndFinishDKG")
	defer span.End()

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return err
	}

	lastCompleted, err := d.store.GetFinished(beaconID)
	if err != nil {
		return err
	}

	executeAndStoreDKG := func() error {
		output, err := d.startDKGExecution(ctx, beaconID, current, &config)
		if err != nil {
			// if the DKG doesn't reach threshold, we must transition to `Failed` instead of
			// returning an error and rolling back
			if !util.ErrorContains(err, "dkg: too many uncompliant new participants") && !util.ErrorContains(err, "dkg abort") {
				return err
			}

			d.log.Errorw("DKG failed as too many nodes were evicted. Storing failed state")

			next, err := current.Failed()
			if err != nil {
				return err
			}

			return d.store.SaveCurrent(beaconID, next)
		}

		finalState, err := current.Complete(output.FinalGroup, output.KeyShare)
		if err != nil {
			return err
		}

		err = d.store.SaveFinished(beaconID, finalState)
		if err != nil {
			return err
		}

		d.completedDKGs <- SharingOutput{
			BeaconID: beaconID,
			Old:      lastCompleted,
			New:      *finalState,
		}

		d.log.Infow("DKG completed successfully!", "beaconID", beaconID, "epoch", finalState.Epoch)
		return nil
	}

	leaveNetwork := func(err error) {
		d.log.Errorw(
			"There was an error during the DKG - we were likely evicted. Will attempt to store failed state",
			"error", err,
		)
		// could this also be a timeout? is that semantically the same as eviction after DKG execution was triggered?
		evictedState, err := current.Evicted()
		if err != nil {
			d.log.Errorw("Failed to store failed state", "error", err)
			return
		}
		err = d.store.SaveCurrent(beaconID, evictedState)
		if err != nil {
			d.log.Errorw("Failed to store failed state", "error", err)
			return
		}
	}

	err = executeAndStoreDKG()
	if err != nil {
		leaveNetwork(err)
	}

	return err
}

func (d *Process) startDKGExecution(
	ctx context.Context,
	beaconID string,
	current *DBState,
	config *dkg.Config,
) (*ExecutionOutput, error) {
	ctx, span := metrics.NewSpan(ctx, "dkg.startDKGExecution")
	defer span.End()
	phaser := dkg.NewTimePhaser(d.config.TimeBetweenDKGPhases)
	go phaser.Start()

	// NewProtocol actually _starts_ the protocol on a goroutine also
	d.lock.Lock()
	broadcaster := d.Executions[beaconID]
	d.lock.Unlock()

	d.log.Info("Starting DKG protocol")
	protocol, err := dkg.NewProtocol(config, broadcaster, phaser, d.config.SkipKeyVerification)
	if err != nil {
		return nil, err
	}

	// wait for the protocol to end and figure out who made it into the final group
	select {
	case result := <-protocol.WaitEnd():
		if result.Error != nil {
			return nil, result.Error
		}

		keypair, err := d.beaconIdentifier.KeypairFor(beaconID)
		if err != nil {
			return nil, err
		}
		share := key.Share{DistKeyShare: *result.Result.Key, Scheme: keypair.Scheme()}

		var finalGroup []dkg.Node
		// the index in the for loop may _not_ align with the index returned in QUAL!
		for _, v := range result.Result.QUAL {
			finalGroup = append(finalGroup, config.NewNodes[v.Index])
		}

		groupFile, err := asGroup(ctx, current, &share, finalGroup)
		if err != nil {
			return nil, err
		}

		output := ExecutionOutput{
			FinalGroup: &groupFile,
			KeyShare:   &share,
		}
		return &output, nil
	case <-time.After(time.Until(current.Timeout)):
		return nil, errors.New("DKG timed out")
	}
}

func asGroup(ctx context.Context, details *DBState, keyShare *key.Share, finalNodes []dkg.Node) (key.Group, error) {
	_, span := metrics.NewSpan(ctx, "dkg.asGroup")
	defer span.End()

	sch, found := crypto.GetSchemeByID(details.SchemeID)
	if !found {
		return key.Group{}, fmt.Errorf("the schemeID for the given group did not exist, scheme: %s", details.SchemeID)
	}

	allSortedParticipants := util.SortedByPublicKey(append(details.Remaining, details.Joining...))

	remainingNodes := make([]*key.Node, len(finalNodes))
	for i, v := range finalNodes {
		mappedNode, err := util.ToKeyNode(int(v.Index), allSortedParticipants[v.Index], keyShare.Scheme)
		if err != nil {
			return key.Group{}, err
		}
		remainingNodes[i] = &mappedNode
	}

	group := key.Group{
		ID:             details.BeaconID,
		Threshold:      int(details.Threshold),
		Period:         details.BeaconPeriod,
		Scheme:         sch,
		CatchupPeriod:  details.CatchupPeriod,
		GenesisTime:    details.GenesisTime.Unix(),
		GenesisSeed:    details.GenesisSeed,
		TransitionTime: details.TransitionTime.Unix(),
		Nodes:          remainingNodes,
		PublicKey:      keyShare.Public(),
	}

	if len(group.GenesisSeed) == 0 {
		group.GenesisSeed = group.Hash()
	}

	return group, nil
}

func (d *Process) initialDKGConfig(current *DBState, keypair *key.Pair, sortedParticipants []*drand.Participant) (*dkg.Config, error) {
	sch := keypair.Scheme()
	newNodes, err := util.TryMapEach[dkg.Node](sortedParticipants, func(index int, participant *drand.Participant) (dkg.Node, error) {
		return util.ToNode(index, participant, sch)
	})
	if err != nil {
		return nil, err
	}

	// although this is an "initial" DKG, we could be a joiner, and we may need to set some things
	// from a prior DKG provided by the network
	var nodes []dkg.Node
	var publicCoeffs []kyber.Point
	var oldThreshold = 0
	if current.FinalGroup != nil {
		nodes = current.FinalGroup.DKGNodes()
		publicCoeffs = current.FinalGroup.PublicKey.Coefficients
		oldThreshold = current.FinalGroup.Threshold
	}

	suite := sch.KeyGroup.(dkg.Suite)
	return &dkg.Config{
		Suite:          suite,
		Longterm:       keypair.Key,
		OldNodes:       nodes,
		NewNodes:       newNodes,
		PublicCoeffs:   publicCoeffs,
		OldThreshold:   oldThreshold,
		Share:          nil,
		Threshold:      int(current.Threshold),
		Reader:         nil,
		UserReaderOnly: false,
		FastSync:       true,
		Nonce:          nonceFor(current),
		Auth:           schnorr.NewScheme(suite),
		Log:            d.log,
	}, nil
}

func (d *Process) reshareDKGConfig(
	current, previous *DBState,
	keypair *key.Pair,
	sortedParticipants []*drand.Participant,
) (*dkg.Config, error) {
	if previous == nil {
		return nil, errors.New("cannot reshare with a nil previous DKG state")
	}

	newNodes, err := util.TryMapEach[dkg.Node](sortedParticipants, func(index int, participant *drand.Participant) (dkg.Node, error) {
		return util.ToNode(index, participant, keypair.Scheme())
	})
	if err != nil {
		return nil, err
	}

	suite := keypair.Scheme().KeyGroup.(dkg.Suite)
	return &dkg.Config{
		Suite:          suite,
		Longterm:       keypair.Key,
		OldNodes:       previous.FinalGroup.DKGNodes(),
		NewNodes:       newNodes,
		PublicCoeffs:   previous.FinalGroup.PublicKey.Coefficients,
		Share:          &previous.KeyShare.DistKeyShare,
		Threshold:      int(current.Threshold),
		OldThreshold:   int(previous.Threshold),
		Reader:         nil,
		UserReaderOnly: false,
		FastSync:       true,
		Nonce:          nonceFor(current),
		Auth:           schnorr.NewScheme(suite),
		Log:            d.log,
	}, nil
}

func nonceFor(state *DBState) []byte {
	h := sha256.New()
	_ = binary.Write(h, binary.BigEndian, state.Epoch)
	return h.Sum(nil)
}

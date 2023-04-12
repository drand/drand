package dkg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/drand/drand/metrics"
	"github.com/drand/kyber"

	"github.com/drand/drand/crypto"

	"github.com/drand/drand/common"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/util"
	"github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/sign/schnorr"
)

func (d *DKGProcess) setupDKG(ctx context.Context, beaconID string) (*dkg.Config, error) {
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
	d.lock.Lock()
	d.Executions[beaconID] = board
	d.lock.Unlock()

	return config, nil
}

// this is done rarely and is a shared object: no good reason not to use a clone (and it makes the race checker happy :))
//
//nolint:gocritic
func (d *DKGProcess) executeAndFinishDKG(ctx context.Context, beaconID string, config dkg.Config) error {
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
			return err
		}

		finalState, err := current.Complete(output.FinalGroup, output.KeyShare)
		if err != nil {
			return err
		}

		d.completedDKGs <- SharingOutput{
			BeaconID: beaconID,
			Old:      lastCompleted,
			New:      *finalState,
		}

		err = d.store.SaveFinished(beaconID, finalState)
		if err != nil {
			return err
		}

		d.log.Infow("DKG completed successfully!", "beaconID", beaconID, "epoch", finalState.Epoch)
		return nil
	}

	leaveNetwork := func(err error) {
		d.log.Errorw("There was an error during the DKG - we were likely evicted. Will attempt to store failed state", "error", err)
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

	return rollbackOnError(executeAndStoreDKG, leaveNetwork)
}

//nolint:lll // This function has a few, explicitly named, paramteres.
func (d *DKGProcess) startDKGExecution(ctx context.Context, beaconID string, current *DBState, config *dkg.Config) (*ExecutionOutput, error) {
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

func asGroup(_ context.Context, details *DBState, keyShare *key.Share, finalNodes []dkg.Node) (key.Group, error) {
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

func (d *DKGProcess) initialDKGConfig(current *DBState, keypair *key.Pair, sortedParticipants []*drand.Participant) (*dkg.Config, error) {
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

func (d *DKGProcess) reshareDKGConfig(
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

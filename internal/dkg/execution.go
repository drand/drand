package dkg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/tracer"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/entropy"
	"github.com/drand/drand/v2/internal/metrics"
	"github.com/drand/drand/v2/internal/util"
	drand "github.com/drand/drand/v2/protobuf/dkg"
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

	go func(config *dkg.Config) {
		// wait until the time set by the leader for kicking off the DKG to allow other nodes to get
		// the requisite packets
		select {
		case <-d.close:
			return
		case <-time.After(time.Until(executionStartTime)):
			err := d.executeAndFinishDKG(ctx, beaconID, config)
			if err != nil {
				d.log.Errorw("there was an error during the DKG!", "beaconID", beaconID, "error", err)
			}
		}
	}(dkgConfig)
	return nil
}

func (d *Process) setupDKG(ctx context.Context, beaconID string) (*dkg.Config, error) {
	ctx, span := tracer.NewSpan(ctx, "dkg.setupDKG")
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
func (d *Process) executeAndFinishDKG(ctx context.Context, beaconID string, config *dkg.Config) error {
	ctx, span := tracer.NewSpan(ctx, "dkg.executeAndFinishDKG")
	defer span.End()

	// Clear the entropy source environment variable when the function returns
	// This ensures it's not used in subsequent DKG executions unless explicitly set again
	if entropySource := os.Getenv("DRAND_ENTROPY_SOURCE"); entropySource != "" {
		defer os.Unsetenv("DRAND_ENTROPY_SOURCE")
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return err
	}

	lastCompleted, err := d.store.GetFinished(beaconID)
	if err != nil {
		return err
	}

	output, err := d.startDKGExecution(ctx, beaconID, current, config)
	if err != nil {
		dkgErr := err
		d.log.Errorw("DKG failed. Storing failed state")
		// we need to refetch the current state here, as `startDKGExecution` may have changed it
		current, err := d.store.GetCurrent(beaconID)
		if err != nil {
			return errors.Join(dkgErr, err)
		}

		next, err := current.Failed()
		if err != nil {
			return errors.Join(err, dkgErr)
		}

		err = d.store.SaveCurrent(beaconID, next)
		metrics.DKGStateChange(next.BeaconID, next.Epoch, false, uint32(next.State))
		return errors.Join(dkgErr, err)
	}

	finalState, err := current.Complete(output.FinalGroup, output.KeyShare)
	if err != nil {
		return err
	}

	err = d.store.SaveFinished(beaconID, finalState)
	if err != nil {
		return err
	}

	// the `Close()` function of the DKG process could close this channel before we write the results of the DKG to it
	// so let's recover the panic and return an error; it should only happen when the daemon is shutting down anyway
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from panic writing to closed channel: %v", r)
		}
	}()

	select {
	case <-d.close:
		close(d.completedDKGs.Chan())
		return errors.New("daemon was closed before DKG execution")
	case d.completedDKGs.Chan() <- SharingOutput{
		BeaconID: beaconID,
		Old:      lastCompleted,
		New:      *finalState,
	}:
		// we just continue execution after sending
	}

	metrics.DKGStateChange(finalState.BeaconID, finalState.Epoch, false, uint32(finalState.State))
	d.log.Infow("DKG completed successfully!", "beaconID", beaconID, "epoch", finalState.Epoch)

	return nil
}

func (d *Process) startDKGExecution(
	ctx context.Context,
	beaconID string,
	current *DBState,
	config *dkg.Config,
) (*ExecutionOutput, error) {
	ctx, span := tracer.NewSpan(ctx, "dkg.startDKGExecution")
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
	case <-d.close:
		return nil, errors.New("daemon was closed before DKG execution completed")
	case result := <-protocol.WaitEnd():
		if result.Error != nil {
			return nil, result.Error
		}

		var transitionTime int64

		if current.Epoch == 1 {
			transitionTime = current.GenesisTime.Unix()
		} else {
			roundsUntilTransition := 10
			currentRound := common.CurrentRound(time.Now().Unix(), current.BeaconPeriod, current.GenesisTime.Unix())
			transitionTime = common.TimeOfRound(current.BeaconPeriod, current.GenesisTime.Unix(), currentRound+uint64(roundsUntilTransition))
		}
		keypair, err := d.beaconIdentifier.KeypairFor(beaconID)
		if err != nil {
			return nil, err
		}
		share := &key.Share{DistKeyShare: *result.Result.Key, Scheme: keypair.Scheme()}

		var finalGroup []dkg.Node
		// the index in the for loop may _not_ align with the index returned in QUAL!
		for _, v := range result.Result.QUAL {
			finalGroup = append(finalGroup, config.NewNodes[v.Index])
		}

		groupFile, err := asGroup(ctx, current, share, finalGroup, transitionTime)
		if err != nil {
			return nil, err
		}

		output := ExecutionOutput{
			FinalGroup: &groupFile,
			KeyShare:   share,
		}
		return &output, nil
	case <-time.After(time.Until(current.Timeout)):
		return nil, errors.New("DKG timed out")
	}
}

func asGroup(ctx context.Context, details *DBState, keyShare *key.Share, finalNodes []dkg.Node, transitionTime int64) (key.Group, error) {
	_, span := tracer.NewSpan(ctx, "dkg.asGroup")
	defer span.End()

	sch, err := crypto.GetSchemeByID(details.SchemeID)
	if err != nil {
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
		TransitionTime: transitionTime,
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

	// Check if a custom entropy source is specified via environment variable
	var reader io.Reader
	if entropySource := os.Getenv("DRAND_ENTROPY_SOURCE"); entropySource != "" {
		d.log.Infow("Using custom entropy source from environment variable", "source", entropySource)
		reader = entropy.NewScriptReader(entropySource)
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
		Reader:         reader,
		UserReaderOnly: reader != nil, // Only use the user's reader if it's provided
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

	// Check if a custom entropy source is specified via environment variable
	var reader io.Reader
	if entropySource := os.Getenv("DRAND_ENTROPY_SOURCE"); entropySource != "" {
		d.log.Infow("Using custom entropy source from environment variable", "source", entropySource)
		reader = entropy.NewScriptReader(entropySource)
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
		Reader:         reader,
		UserReaderOnly: reader != nil, // Only use the user's reader if it's provided
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

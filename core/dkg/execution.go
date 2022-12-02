package dkg

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/drand/drand/common"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/util"
	"github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/sign/schnorr"
	"time"
)

func (d *DKGProcess) executeDKG(beaconID string, lastCompleted *DKGState, current *DKGState) (*ExecutionOutput, error) {
	d.Lock()
	defer d.Unlock()

	keypair, err := d.beaconIdentifier.KeypairFor(beaconID)
	if err != nil {
		return nil, err
	}
	me, err := util.PublicKeyAsParticipant(keypair.Public)
	if err != nil {
		return nil, err
	}

	if lastCompleted == nil {
		return d.executeInitialDKG(beaconID, keypair, me, current)
	}

	sortedParticipants := util.SortedByPublicKey(append(current.Remaining, current.Joining...))

	newNodes, err := util.TryMapEach[dkg.Node](sortedParticipants, util.ToNode)
	if err != nil {
		return nil, err
	}

	suite := key.KeyGroup.(dkg.Suite)
	config := dkg.Config{
		Suite:          suite,
		Longterm:       keypair.Key,
		OldNodes:       lastCompleted.FinalGroup.DKGNodes(),
		NewNodes:       newNodes,
		PublicCoeffs:   lastCompleted.FinalGroup.PublicKey.Coefficients,
		Share:          (*dkg.DistKeyShare)(lastCompleted.KeyShare),
		Threshold:      int(current.Threshold),
		OldThreshold:   int(lastCompleted.Threshold),
		Reader:         nil,
		UserReaderOnly: false,
		FastSync:       true,
		Nonce:          nonceFor(current),
		Auth:           schnorr.NewScheme(suite),
		Log:            d.log,
	}

	return d.startDKGAndBroadcastExecution(beaconID, me, current, sortedParticipants, config)
}

func (d *DKGProcess) executeInitialDKG(beaconID string, keypair *key.Pair, me *drand.Participant, current *DKGState) (*ExecutionOutput, error) {
	sortedParticipants := util.SortedByPublicKey(append(current.Remaining, current.Joining...))
	newNodes, err := util.TryMapEach[dkg.Node](sortedParticipants, util.ToNode)
	if err != nil {
		return nil, err
	}

	suite := key.KeyGroup.(dkg.Suite)
	config := dkg.Config{
		Suite:          suite,
		Longterm:       keypair.Key,
		OldNodes:       nil,
		NewNodes:       newNodes,
		PublicCoeffs:   nil,
		Share:          nil,
		Threshold:      int(current.Threshold),
		OldThreshold:   0,
		Reader:         nil,
		UserReaderOnly: false,
		FastSync:       true,
		Nonce:          nonceFor(current),
		Auth:           schnorr.NewScheme(suite),
		Log:            d.log,
	}

	return d.startDKGAndBroadcastExecution(beaconID, me, current, sortedParticipants, config)
}

func (d *DKGProcess) startDKGAndBroadcastExecution(beaconID string, me *drand.Participant, current *DKGState, sortedParticipants []*drand.Participant, config dkg.Config) (*ExecutionOutput, error) {
	// create the network over which to send all the DKG packets
	board, err := NewEchoBroadcast(
		d.log,
		common.GetAppVersion(),
		current.BeaconID,
		me.Address,
		sortedParticipants,
		func(p dkg.Packet) error {
			return dkg.VerifyPacketSignature(&config, p)
		})
	if err != nil {
		return nil, err
	}

	// we need some state on the DKG process in order to process any incoming gossip messages from the DKG
	// if other nodes try to send us DKG messages before this is set we're in trouble
	d.executions[beaconID] = board

	timeBetweenDKGPhases := 2 * time.Second
	phaser := dkg.NewTimePhaser(timeBetweenDKGPhases)
	go phaser.Start()

	// NewProtocol actually _starts_ the protocol on a goroutine also
	protocol, err := dkg.NewProtocol(&config, board, phaser, false)
	if err != nil {
		return nil, err
	}

	// wait for the protocol to end and figure out who made it into the final group
	select {
	case result := <-protocol.WaitEnd():
		{
			if result.Error != nil {
				return nil, result.Error
			}

			keyShare := (*key.Share)(result.Result.Key)

			var finalGroup []*drand.Participant
			// the index in the for loop may _not_ align with the index returned in QUAL!
			for _, v := range result.Result.QUAL {
				finalGroup = append(finalGroup, sortedParticipants[v.Index])
			}

			groupFile, err := asGroup(current, keyShare, finalGroup)
			if err != nil {
				return nil, err
			}

			output := ExecutionOutput{
				FinalGroup: &groupFile,
				KeyShare:   keyShare,
			}
			return &output, nil
		}
	case <-time.After(d.config.Timeout):
		{
			return nil, errors.New("DKG timed out")
		}
	}
}

func asGroup(details *DKGState, keyShare *key.Share, finalParticipants []*drand.Participant) (key.Group, error) {
	scheme, found := scheme.GetSchemeByID(details.SchemeID)
	if !found {
		return key.Group{}, fmt.Errorf("the schemeID for the given group did not exist, scheme: %s", details.SchemeID)
	}

	finalGroupSorted := util.SortedByPublicKey(finalParticipants)
	participantToKeyNode := func(index int, participant *drand.Participant) (*key.Node, error) {
		r, err := util.ToKeyNode(index, participant)
		if err != nil {
			return nil, err
		}
		return &r, nil
	}
	nodes, err := util.TryMapEach[*key.Node](finalGroupSorted, participantToKeyNode)
	if err != nil {
		return key.Group{}, err
	}

	group := key.Group{
		ID:             details.BeaconID,
		Threshold:      int(details.Threshold),
		Period:         details.BeaconPeriod,
		Scheme:         scheme,
		CatchupPeriod:  details.CatchupPeriod,
		GenesisTime:    details.GenesisTime.Unix(),
		GenesisSeed:    details.GenesisSeed,
		TransitionTime: details.TransitionTime.Unix(),
		Nodes:          nodes,
		PublicKey:      keyShare.Public(),
	}

	if len(group.GenesisSeed) == 0 {
		group.GenesisSeed = group.Hash()
	}

	return group, nil
}

func nonceFor(state *DKGState) []byte {
	h := sha256.New()
	_ = binary.Write(h, binary.BigEndian, state.Epoch)
	return h.Sum(nil)
}

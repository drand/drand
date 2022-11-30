package dkg

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"github.com/drand/drand/common"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/util"
	"github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/sign/schnorr"
	"time"
)

func (d *DKGProcess) executeDKG(beaconID string, lastCompleted *DKGState, current *DKGState) (*ExecutionOutput, error) {
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

	oldNodes, err := util.TryMapEach[dkg.Node](util.SortedByPublicKey(lastCompleted.FinalGroup), util.ToNode)
	if err != nil {
		return nil, err
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
		OldNodes:       oldNodes,
		NewNodes:       newNodes,
		PublicCoeffs:   nil,
		Share:          nil,
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
	d.executions[beaconID] = board
	defer func() {
		delete(d.executions, beaconID)
	}()

	timeBetweenDKGPhases := 1 * time.Second
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
			var finalGroup []*drand.Participant
			for i := range result.Result.QUAL {
				finalGroup = append(finalGroup, sortedParticipants[i])
			}

			output := ExecutionOutput{
				FinalGroup: finalGroup,
				KeyShare:   result.Result.Key,
			}
			return &output, nil
		}
	case <-time.After(d.config.Timeout):
		{
			return nil, errors.New("DKG timed out")
		}
	}
}

func nonceFor(state *DKGState) []byte {
	h := sha256.New()
	_ = binary.Write(h, binary.BigEndian, state.Epoch)
	return h.Sum(nil)
}

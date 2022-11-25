package dkg

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"github.com/drand/drand/common"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/sign/schnorr"
	"sort"
	"time"
)

type DKGOutput struct {
	finalGroup []*drand.Participant
	keyShare   *dkg.DistKeyShare
}

func (d *DKGProcess) executeDKG(beaconID string, lastCompleted *DKGState, current *DKGState) (*DKGOutput, error) {
	keypair, err := d.beaconIdentifier.KeypairFor(beaconID)
	if err != nil {
		return nil, err
	}
	me, err := publicKeyAsParticipant(keypair.Public)
	if err != nil {
		return nil, err
	}

	if lastCompleted == nil {
		return d.executeInitialDKG(beaconID, keypair, me, current)
	}
	oldNodes, err := TryMapEach[dkg.Node](sortedByPublicKey(lastCompleted.FinalGroup), toNode)
	if err != nil {
		return nil, err
	}

	sortedParticipants := sortedByPublicKey(append(current.Remaining, current.Joining...))
	newNodes, err := TryMapEach[dkg.Node](sortedParticipants, toNode)
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

	return d.createBroadcastAndStartDKG(beaconID, me, current, sortedParticipants, config)
}

func (d *DKGProcess) executeInitialDKG(beaconID string, keypair *key.Pair, me *drand.Participant, current *DKGState) (*DKGOutput, error) {
	sortedParticipants := sortedByPublicKey(append(current.Remaining, current.Joining...))
	newNodes, err := TryMapEach[dkg.Node](sortedParticipants, toNode)
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

	return d.createBroadcastAndStartDKG(beaconID, me, current, sortedParticipants, config)
}

func (d *DKGProcess) createBroadcastAndStartDKG(beaconID string, me *drand.Participant, current *DKGState, sortedParticipants []*drand.Participant, config dkg.Config) (*DKGOutput, error) {
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

	// ewwww state
	d.executions[beaconID] = board
	if err != nil {
		return nil, err
	}

	phaser := dkg.NewTimePhaser(1 * time.Second)
	// NewProtocol actually _starts_ the protocol on a goroutine also - could this be a source of timing issues?
	protocol, err := dkg.NewProtocol(&config, board, phaser, false)
	if err != nil {
		return nil, err
	}

	go phaser.Start()

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
			output := DKGOutput{
				finalGroup: finalGroup,
				keyShare:   result.Result.Key,
			}
			return &output, nil
		}
	case <-time.After(1 * time.Minute):
		{
			return nil, errors.New("DKG timed out")
		}
	}
}

func sortedByPublicKey(arr []*drand.Participant) []*drand.Participant {
	out := arr
	sort.Slice(out, func(i, j int) bool {
		return string(arr[i].PubKey) < string(arr[j].PubKey)
	})
	return out
}

func toNode(index int, participant *drand.Participant) (dkg.Node, error) {
	public, err := pkToPoint(participant.PubKey)
	if err != nil {
		return dkg.Node{}, err
	}
	return dkg.Node{
		Public: public,
		Index:  uint32(index),
	}, nil
}

func pkToPoint(pk []byte) (kyber.Point, error) {
	point := key.KeyGroup.Point()
	if err := point.UnmarshalBinary(pk); err != nil {
		return nil, err
	}
	return point, nil
}

func TryMapEach[T any](arr []*drand.Participant, fn func(index int, participant *drand.Participant) (T, error)) ([]T, error) {
	out := make([]T, len(arr))
	for i, participant := range arr {
		p := participant
		result, err := fn(i, p)
		if err != nil {
			return nil, err
		}
		out[i] = result
	}
	return out, nil
}

func nonceFor(state *DKGState) []byte {
	h := sha256.New()
	_ = binary.Write(h, binary.BigEndian, state.Epoch)
	return h.Sum(nil)
}

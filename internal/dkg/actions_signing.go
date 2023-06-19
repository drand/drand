package dkg

import (
	"errors"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/internal/util"

	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (d *Process) signMessage(beaconID, messageType string, proposal *drand.ProposalTerms) (*drand.DKGMetadata, error) {
	kp, err := d.beaconIdentifier.KeypairFor(beaconID)
	if err != nil {
		return nil, err
	}

	sig, err := kp.Scheme().AuthScheme.Sign(kp.Key, messageForProto(proposal, messageType, beaconID))
	if err != nil {
		return nil, err
	}
	return &drand.DKGMetadata{
		BeaconID:  beaconID,
		Address:   kp.Public.Address(),
		Signature: sig,
	}, nil
}

func (d *Process) verifyMessage(messageType string, metadata *drand.DKGMetadata, proposal *drand.ProposalTerms) error {
	participants := util.Concat(proposal.Remaining, proposal.Joining)
	// signing is done before the metadata is attached, so we must remove it before we perform verification
	proposal.Metadata = nil

	// find the participant the signature is allegedly from
	var p *drand.Participant
	for _, participant := range participants {
		if participant.Address == metadata.Address {
			p = participant
			break
		}
	}
	if p == nil {
		return errors.New("no such participant")
	}

	// get the scheme for the network so we can correctly unmarshal the public key
	kp, err := d.beaconIdentifier.KeypairFor(metadata.BeaconID)
	if err != nil {
		return err
	}

	// use that scheme to verify the message came from the alleged author
	pubPoint := kp.Scheme().KeyGroup.Point()
	err = pubPoint.UnmarshalBinary(p.PubKey)
	if err != nil {
		return key.ErrInvalidKeyScheme
	}
	return kp.Scheme().AuthScheme.Verify(pubPoint, messageForProto(proposal, messageType, metadata.BeaconID), metadata.Signature)
}

func messageForProto(proposal *drand.ProposalTerms, messageType, beaconID string) []byte {
	return []byte(proposal.String() + messageType + beaconID)
}

// used for determining the message that was signed for verifying packet authenticity
func termsFromState(state *DBState) *drand.ProposalTerms {
	return &drand.ProposalTerms{
		BeaconID:             state.BeaconID,
		Threshold:            state.Threshold,
		Epoch:                state.Epoch,
		SchemeID:             state.SchemeID,
		BeaconPeriodSeconds:  uint32(state.BeaconPeriod.Seconds()),
		CatchupPeriodSeconds: uint32(state.CatchupPeriod.Seconds()),
		GenesisTime:          timestamppb.New(state.GenesisTime),
		GenesisSeed:          state.GenesisSeed,
		TransitionTime:       timestamppb.New(state.TransitionTime),
		Timeout:              timestamppb.New(state.Timeout),
		Leader:               state.Leader,
		Joining:              state.Joining,
		Remaining:            state.Remaining,
		Leaving:              state.Leaving,
	}
}

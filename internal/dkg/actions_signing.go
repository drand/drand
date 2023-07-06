package dkg

import (
	"errors"
	"google.golang.org/protobuf/proto"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/drand/drand/common/key"
	"github.com/drand/drand/internal/util"
	"github.com/drand/drand/protobuf/drand"
)

func (d *Process) signMessage(
	beaconID string,
	packet *drand.GossipPacket,
	proposal *drand.ProposalTerms,
) (*drand.GossipMetadata, error) {
	kp, err := d.beaconIdentifier.KeypairFor(beaconID)
	if err != nil {
		return nil, err
	}

	sig, err := kp.Scheme().AuthScheme.Sign(kp.Key, messageForProto(proposal, packet, beaconID))
	if err != nil {
		return nil, err
	}
	return &drand.GossipMetadata{
		BeaconID:  beaconID,
		Address:   kp.Public.Address(),
		Signature: sig,
	}, nil
}

func (d *Process) verifyMessage(packet *drand.GossipPacket, metadata *drand.GossipMetadata, proposal *drand.ProposalTerms) error {
	participants := util.Concat(proposal.Remaining, proposal.Joining)
	// signing is done before the metadata is attached, so we must remove it before we perform verification

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

	// get the scheme for the network, so we can correctly unmarshal the public key
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

	return kp.Scheme().AuthScheme.Verify(pubPoint, messageForProto(proposal, packet, metadata.BeaconID), metadata.Signature)
}

func messageForProto(proposal *drand.ProposalTerms, packet *drand.GossipPacket, beaconID string) []byte {
	// we remove the metadata for verification of the packet, as the signer hasn't created the metadta
	// upon signing
	packetWithoutMetadata := proto.Clone(packet).(*drand.GossipPacket)
	packetWithoutMetadata.Metadata = nil
	return []byte(proposal.String() + packetWithoutMetadata.String() + beaconID)
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

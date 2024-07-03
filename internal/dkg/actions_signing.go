package dkg

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/internal/util"
	drand "github.com/drand/drand/v2/protobuf/dkg"
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

	sig, err := kp.Scheme().AuthScheme.Sign(kp.Key, messageForSigning(beaconID, packet, proposal))
	if err != nil {
		return nil, err
	}
	return &drand.GossipMetadata{
		BeaconID:  beaconID,
		Address:   kp.Public.Address(),
		Signature: sig,
	}, nil
}

func (d *Process) verifyMessage(packet *drand.GossipPacket, proposal *drand.ProposalTerms) error {
	beaconID := packet.GetMetadata().GetBeaconID()
	d.log.Debugw("Verifying DKG packet", "beaconID", beaconID, "from", packet.GetMetadata().GetAddress())

	participants := util.Concat(proposal.GetRemaining(), proposal.GetJoining())
	// signing is done before the metadata is attached, so we must remove it before we perform verification

	// find the participant the signature is allegedly from
	var p *drand.Participant
	for _, participant := range participants {
		if participant.GetAddress() == packet.GetMetadata().GetAddress() {
			p = participant
			break
		}
	}
	if p == nil {
		return errors.New("no such participant")
	}

	// get the scheme for the network, so we can correctly unmarshal the public key
	kp, err := d.beaconIdentifier.KeypairFor(beaconID)
	if err != nil {
		return err
	}

	// use that scheme to verify the message came from the alleged author
	pubPoint := kp.Scheme().KeyGroup.Point()
	err = pubPoint.UnmarshalBinary(p.Key)
	if err != nil {
		return fmt.Errorf("unable to verify packet allegedly from %s: %w", packet.GetMetadata().GetAddress(), key.ErrInvalidKeyScheme)
	}

	// we need to copy here or the GC/compiler does something weird
	sig := make([]byte, len(packet.GetMetadata().GetSignature()))
	copy(sig, packet.GetMetadata().GetSignature())

	msg := messageForSigning(beaconID, packet, proposal)
	err = kp.Scheme().AuthScheme.Verify(pubPoint, msg, sig)
	if err != nil {
		return fmt.Errorf("error verifying '%s' packet with msg '%s' and metadata '%s': %w ", packetName(packet), hex.EncodeToString(msg), packet.GetMetadata(), err)
	}
	return nil
}

func messageForSigning(beaconID string, packet *drand.GossipPacket, proposal *drand.ProposalTerms) []byte {
	// bytes.Buffer Writes are always returning nil errors, no need to check them
	var ret bytes.Buffer
	// we validate the packet type
	ret.WriteString("beaconID:" + beaconID + "\n")

	switch t := packet.Packet.(type) {
	case *drand.GossipPacket_Proposal:
		ret.WriteString("Proposal:")
		ret.WriteString(t.Proposal.GetBeaconID() + "\n")
		ret.Write(binary.LittleEndian.AppendUint32([]byte{}, t.Proposal.GetEpoch()))
		ret.WriteString("\nLeader:" + t.Proposal.GetLeader().GetAddress() + "\n")
		ret.Write(t.Proposal.GetLeader().GetSignature())
		// the rest of a proposal packet is verified by processing below the proposal terms after having applied the proposal
	case *drand.GossipPacket_Accept:
		ret.WriteString("Accepted:" + t.Accept.GetAcceptor().GetAddress() + "\n")
	case *drand.GossipPacket_Reject:
		ret.WriteString("Rejected:" + t.Reject.GetRejector().GetAddress() + "\n")
	case *drand.GossipPacket_Abort:
		ret.WriteString("Aborted:" + t.Abort.GetReason() + "\n")
	case *drand.GossipPacket_Execute:
		enc, _ := t.Execute.GetTime().AsTime().MarshalBinary()
		ret.WriteString("Execute:")
		ret.Write(enc)
	case *drand.GossipPacket_Dkg:
		ret.WriteString("Gossip packet")
	default:
		// this is impossible in theory: there are checks erroring out much earlier
		// so if we get an unknown packet type we make sure signature verification fails
		ensureFailure := make([]byte, 64)
		rand.Reader.Read(ensureFailure)
		ret.Write(ensureFailure)
	}

	ret.WriteString("Proposal:\n")
	// respecting the Protobuf ordering as per dkg_control.proto
	ret.WriteString(proposal.GetBeaconID() + "\n")
	ret.Write(binary.LittleEndian.AppendUint32([]byte{}, proposal.GetEpoch()))
	ret.WriteString("\nLeader:" + proposal.GetLeader().GetAddress() + "\n")
	ret.Write(proposal.GetLeader().GetSignature())
	ret.Write(binary.LittleEndian.AppendUint32([]byte{}, proposal.GetThreshold()))

	encTimeout, _ := proposal.GetTimeout().AsTime().MarshalBinary()
	ret.Write(encTimeout)

	ret.Write(binary.LittleEndian.AppendUint32([]byte{}, proposal.GetCatchupPeriodSeconds()))
	ret.Write(binary.LittleEndian.AppendUint32([]byte{}, proposal.GetBeaconPeriodSeconds()))
	ret.WriteString("\nScheme: " + proposal.GetSchemeID() + "\n")

	encGenesis, _ := proposal.GetGenesisTime().AsTime().MarshalBinary()
	ret.Write(encGenesis)

	for _, p := range proposal.GetJoining() {
		ret.WriteString("\nJoiner:" + p.GetAddress() + "\nSig:")
		ret.Write(p.GetSignature())
	}

	for _, p := range proposal.GetRemaining() {
		ret.WriteString("\nRemainer:" + p.GetAddress() + "\nSig:")
		ret.Write(p.GetSignature())
	}

	for _, p := range proposal.GetLeaving() {
		ret.WriteString("\nLeaver:" + p.GetAddress() + "\nSig:")
		ret.Write(p.GetSignature())
	}

	return ret.Bytes()
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
		Timeout:              timestamppb.New(state.Timeout),
		Leader:               state.Leader,
		Joining:              state.Joining,
		Remaining:            state.Remaining,
		Leaving:              state.Leaving,
	}
}

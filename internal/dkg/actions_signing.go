package dkg

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/util"
	drand "github.com/drand/drand/v2/protobuf/dkg"
	"google.golang.org/protobuf/proto"
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
		return fmt.Errorf("error verifying '%s' packet with msg '%s' and metadata '%s': %w ",
			packetName(packet),
			hex.EncodeToString(msg),
			packet.GetMetadata(),
			err)
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
		//nolint:mnd
		ensureFailure := make([]byte, 16)
		_, _ = rand.Reader.Read(ensureFailure)
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

	// Write durations as nanoseconds for deterministic hashing
	catchupPeriod := proposal.GetCatchupPeriod()
	if catchupPeriod != nil && catchupPeriod.IsValid() {
		nanoCatchup := catchupPeriod.AsDuration().Nanoseconds()
		ret.Write(binary.LittleEndian.AppendUint64([]byte{}, uint64(nanoCatchup)))
	} else {
		ret.Write(binary.LittleEndian.AppendUint64([]byte{}, 0)) // Write 0 if nil/invalid
	}

	beaconPeriod := proposal.GetBeaconPeriod()
	if beaconPeriod != nil && beaconPeriod.IsValid() {
		nanoPeriod := beaconPeriod.AsDuration().Nanoseconds()
		ret.Write(binary.LittleEndian.AppendUint64([]byte{}, uint64(nanoPeriod)))
	} else {
		ret.Write(binary.LittleEndian.AppendUint64([]byte{}, 0)) // Write 0 if nil/invalid
	}

	ret.WriteString("\nScheme: " + proposal.GetSchemeID() + "\n")

	encGenesis, _ := proposal.GetGenesisTime().AsTime().MarshalBinary()
	ret.Write(encGenesis)
	ret.Write(proposal.GetGenesisSeed()) // Add GenesisSeed to hash

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
	var catchupProto *durationpb.Duration
	if state.CatchupPeriod > 0 {
		catchupProto = durationpb.New(state.CatchupPeriod)
	}
	var beaconProto *durationpb.Duration
	if state.BeaconPeriod > 0 {
		beaconProto = durationpb.New(state.BeaconPeriod)
	}
	return &drand.ProposalTerms{
		BeaconID:      state.BeaconID,
		Threshold:     state.Threshold,
		Epoch:         state.Epoch,
		SchemeID:      state.SchemeID,
		BeaconPeriod:  beaconProto,
		CatchupPeriod: catchupProto,
		GenesisTime:   timestamppb.New(state.GenesisTime),
		GenesisSeed:   state.GenesisSeed,
		Timeout:       timestamppb.New(state.Timeout),
		Leader:        state.Leader,
		Joining:       state.Joining,
		Remaining:     state.Remaining,
		Leaving:       state.Leaving,
	}
}

// CreateDealBundleHash creates the hash that needs to be signed by the dealer.
func CreateDealBundleHash(bundle *drand.DealBundle, schemeID string, epoch uint32) ([]byte, error) {
	sch, err := crypto.GetSchemeByID(schemeID)
	if err != nil {
		return nil, fmt.Errorf("actions_signing: invalid scheme ID '%s' for hashing: %w", schemeID, err)
	}
	h := sch.IdentityHash()
	if h == nil {
		return nil, fmt.Errorf("actions_signing: hasher is nil for scheme ID '%s'", schemeID)
	}
	_, _ = h.Write([]byte(schemeID))
	_ = binary.Write(h, binary.LittleEndian, epoch)
	_ = binary.Write(h, binary.LittleEndian, bundle.DealerIndex)
	for _, c := range bundle.Commits {
		_, _ = h.Write(c)
	}
	for _, d := range bundle.Deals {
		dealBytes, err := proto.Marshal(d)
		if err != nil {
			return nil, fmt.Errorf("actions_signing: failed to marshal deal for hashing: %w", err)
		}
		_, _ = h.Write(dealBytes)
	}
	_, _ = h.Write(bundle.SessionId)
	return h.Sum(nil), nil
}

package core

import (
	"errors"
	"fmt"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	pdkg "github.com/drand/drand/protobuf/crypto/dkg"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share/dkg"
)

func beaconToProto(b *chain.Beacon) *drand.PublicRandResponse {
	return &drand.PublicRandResponse{
		Round:             b.Round,
		Signature:         b.Signature,
		PreviousSignature: b.PreviousSig,
		Randomness:        b.Randomness(),
	}
}

func protoToDKGPacket(d *pdkg.Packet) (dkg.Packet, error) {
	switch packet := d.GetBundle().(type) {
	case *pdkg.Packet_Deal:
		return protoToDeal(packet.Deal)
	case *pdkg.Packet_Response:
		return protoToResp(packet.Response), nil
	case *pdkg.Packet_Justification:
		return protoToJustif(packet.Justification)
	default:
		return nil, errors.New("unknown packet")
	}
}

func dkgPacketToProto(p dkg.Packet) (*pdkg.Packet, error) {
	switch inner := p.(type) {
	case *dkg.DealBundle:
		return dealToProto(inner), nil
	case *dkg.ResponseBundle:
		return respToProto(inner), nil
	case *dkg.JustificationBundle:
		return justifToProto(inner), nil
	default:
		return nil, errors.New("invalid dkg packet")
	}
}

func protoToDeal(d *pdkg.DealBundle) (*dkg.DealBundle, error) {
	bundle := new(dkg.DealBundle)
	bundle.DealerIndex = d.DealerIndex
	publics := make([]kyber.Point, 0, len(d.Commits))
	for _, c := range d.Commits {
		coeff := key.KeyGroup.Point()
		if err := coeff.UnmarshalBinary(c); err != nil {
			return nil, fmt.Errorf("invalid public coeff:%s", err)
		}
		publics = append(publics, coeff)
	}
	bundle.Public = publics
	deals := make([]dkg.Deal, 0, len(d.Deals))
	for _, dd := range d.Deals {
		deal := dkg.Deal{
			EncryptedShare: dd.EncryptedShare,
			ShareIndex:     dd.ShareIndex,
		}
		deals = append(deals, deal)
	}
	bundle.Deals = deals
	bundle.SessionID = d.SessionId
	bundle.Signature = d.Signature
	return bundle, nil
}

func protoToResp(r *pdkg.ResponseBundle) *dkg.ResponseBundle {
	resp := new(dkg.ResponseBundle)
	resp.ShareIndex = r.ShareIndex
	resp.Responses = make([]dkg.Response, 0, len(r.Responses))
	for _, rr := range r.Responses {
		response := dkg.Response{
			DealerIndex: rr.DealerIndex,
			Status:      rr.Status,
		}
		resp.Responses = append(resp.Responses, response)
	}
	resp.SessionID = r.SessionId
	resp.Signature = r.Signature
	return resp
}

func protoToJustif(j *pdkg.JustificationBundle) (*dkg.JustificationBundle, error) {
	just := new(dkg.JustificationBundle)
	just.DealerIndex = j.DealerIndex
	just.Justifications = make([]dkg.Justification, len(j.Justifications))
	for i, j := range j.Justifications {
		share := key.KeyGroup.Scalar()
		if err := share.UnmarshalBinary(j.Share); err != nil {
			return nil, fmt.Errorf("invalid share: %s", err)
		}
		justif := dkg.Justification{
			ShareIndex: j.ShareIndex,
			Share:      share,
		}
		just.Justifications[i] = justif
	}
	just.SessionID = j.SessionId
	just.Signature = j.Signature
	return just, nil
}

func dealToProto(d *dkg.DealBundle) *pdkg.Packet {
	packet := new(pdkg.Packet)
	bundle := new(pdkg.DealBundle)
	bundle.DealerIndex = d.DealerIndex
	bundle.Deals = make([]*pdkg.Deal, len(d.Deals))
	for i, deal := range d.Deals {
		pdeal := &pdkg.Deal{
			ShareIndex:     deal.ShareIndex,
			EncryptedShare: deal.EncryptedShare,
		}
		bundle.Deals[i] = pdeal
	}

	bundle.Commits = make([][]byte, len(d.Public))
	for i, coeff := range d.Public {
		cbuff, _ := coeff.MarshalBinary()
		bundle.Commits[i] = cbuff
	}
	bundle.Signature = d.Signature
	bundle.SessionId = d.SessionID
	packet.Bundle = &pdkg.Packet_Deal{Deal: bundle}
	return packet
}

func respToProto(r *dkg.ResponseBundle) *pdkg.Packet {
	packet := new(pdkg.Packet)
	bundle := new(pdkg.ResponseBundle)
	bundle.ShareIndex = r.ShareIndex
	bundle.Responses = make([]*pdkg.Response, len(r.Responses))
	for i, resp := range r.Responses {
		presp := &pdkg.Response{
			DealerIndex: resp.DealerIndex,
			Status:      resp.Status,
		}
		bundle.Responses[i] = presp
	}
	bundle.SessionId = r.SessionID
	bundle.Signature = r.Signature
	packet.Bundle = &pdkg.Packet_Response{Response: bundle}
	return packet
}

func justifToProto(j *dkg.JustificationBundle) *pdkg.Packet {
	packet := new(pdkg.Packet)
	bundle := new(pdkg.JustificationBundle)
	bundle.DealerIndex = j.DealerIndex
	bundle.Justifications = make([]*pdkg.Justification, len(j.Justifications))
	for i, just := range j.Justifications {
		shareBuff, _ := just.Share.MarshalBinary()
		pjust := &pdkg.Justification{
			ShareIndex: just.ShareIndex,
			Share:      shareBuff,
		}
		bundle.Justifications[i] = pjust
	}
	bundle.SessionId = j.SessionID
	bundle.Signature = j.Signature
	packet.Bundle = &pdkg.Packet_Justification{Justification: bundle}
	return packet
}

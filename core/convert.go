package core

import (
	"fmt"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
	pdkg "github.com/drand/drand/protobuf/crypto/dkg"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share/dkg"
)

func beaconToProto(b *beacon.Beacon) *drand.PublicRandResponse {
	return &drand.PublicRandResponse{
		Round:             b.Round,
		Signature:         b.Signature,
		PreviousSignature: b.PreviousSig,
		Randomness:        b.Randomness(),
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
	return resp
}

func protoToJustif(j *pdkg.JustifBundle) (*dkg.JustificationBundle, error) {
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
	return just, nil
}

func dealToProto(d *dkg.AuthDealBundle) *pdkg.Packet {
	packet := new(pdkg.Packet)
	bundle := new(pdkg.DealBundle)
	bundle.DealerIndex = d.Bundle.DealerIndex
	bundle.Deals = make([]*pdkg.Deal, len(d.Bundle.Deals))
	for i, deal := range d.Bundle.Deals {
		pdeal := &pdkg.Deal{
			ShareIndex:     deal.ShareIndex,
			EncryptedShare: deal.EncryptedShare,
		}
		bundle.Deals[i] = pdeal
	}

	bundle.Commits = make([][]byte, len(d.Bundle.Public))
	for i, coeff := range d.Bundle.Public {
		cbuff, _ := coeff.MarshalBinary()
		bundle.Commits[i] = cbuff
	}
	packet.Signature = d.Signature
	packet.Bundle = &pdkg.Packet_Deal{Deal: bundle}
	return packet
}

func respToProto(r *dkg.AuthResponseBundle) *pdkg.Packet {
	packet := new(pdkg.Packet)
	bundle := new(pdkg.ResponseBundle)
	bundle.ShareIndex = r.Bundle.ShareIndex
	bundle.Responses = make([]*pdkg.Response, len(r.Bundle.Responses))
	for i, resp := range r.Bundle.Responses {
		presp := &pdkg.Response{
			DealerIndex: resp.DealerIndex,
			Status:      resp.Status,
		}
		bundle.Responses[i] = presp
	}
	packet.Signature = r.Signature
	packet.Bundle = &pdkg.Packet_Response{Response: bundle}
	return packet
}

func justifToProto(j *dkg.AuthJustifBundle) *pdkg.Packet {
	packet := new(pdkg.Packet)
	bundle := new(pdkg.JustifBundle)
	bundle.DealerIndex = j.Bundle.DealerIndex
	bundle.Justifications = make([]*pdkg.Justification, len(j.Bundle.Justifications))
	for i, just := range j.Bundle.Justifications {
		shareBuff, _ := just.Share.MarshalBinary()
		pjust := &pdkg.Justification{
			ShareIndex: just.ShareIndex,
			Share:      shareBuff,
		}
		bundle.Justifications[i] = pjust
	}
	packet.Signature = j.Signature
	packet.Bundle = &pdkg.Packet_Justification{Justification: bundle}
	return packet
}

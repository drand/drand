package core

import (
	"github.com/drand/drand/chain"
	"github.com/drand/drand/protobuf/drand"
)

func beaconToProto(b *chain.Beacon) *drand.PublicRandResponse {
	return &drand.PublicRandResponse{
		Round:             b.Round,
		Signature:         b.Signature,
		PreviousSignature: b.PreviousSig,
		Randomness:        b.Randomness(),
	}
}

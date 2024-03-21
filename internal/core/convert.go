package core

import (
	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/protobuf/drand"
)

func beaconToProto(b *common.Beacon) *drand.PublicRandResponse {
	return &drand.PublicRandResponse{
		Round:             b.Round,
		Signature:         b.Signature,
		PreviousSignature: b.PreviousSig,
		Randomness:        b.Randomness(),
	}
}

package beacon

import (
	"github.com/drand/drand/protobuf/drand"
	proto "github.com/drand/drand/protobuf/drand"
)

func beaconToSyncResponse(b *Beacon) *drand.SyncResponse {
	return &proto.SyncResponse{
		PreviousRound: b.PreviousRound,
		PreviousSig:   b.PreviousSig,
		Round:         b.Round,
		Signature:     b.Signature,
	}
}

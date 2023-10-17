package beacon

import (
	"github.com/drand/drand/common"
	drand "github.com/drand/drand/protobuf/common"
	proto "github.com/drand/drand/protobuf/drand"
)

func beaconToProto(b *common.Beacon, beaconID string) *proto.BeaconPacket {
	return &proto.BeaconPacket{
		PreviousSignature: b.PreviousSig,
		Round:             b.Round,
		Signature:         b.Signature,
		Metadata:          &drand.Metadata{BeaconID: beaconID},
	}
}

func protoToBeacon(p *proto.BeaconPacket) *common.Beacon {
	return &common.Beacon{
		Round:       p.GetRound(),
		Signature:   p.GetSignature(),
		PreviousSig: p.GetPreviousSignature(),
	}
}

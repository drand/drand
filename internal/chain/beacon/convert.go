package beacon

import (
	common2 "github.com/drand/drand/common"
	"github.com/drand/drand/protobuf/common"
	proto "github.com/drand/drand/protobuf/drand"
)

func beaconToProto(b *common2.Beacon, beaconID string) *proto.BeaconPacket {
	return &proto.BeaconPacket{
		PreviousSignature: b.PreviousSig,
		Round:             b.Round,
		Signature:         b.Signature,
		Metadata:          &common.Metadata{BeaconID: beaconID},
	}
}

func protoToBeacon(p *proto.BeaconPacket) *common2.Beacon {
	return &common2.Beacon{
		Round:       p.GetRound(),
		Signature:   p.GetSignature(),
		PreviousSig: p.GetPreviousSignature(),
	}
}

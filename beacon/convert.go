package beacon

import (
	proto "github.com/drand/drand/protobuf/drand"
)

func beaconToProto(b *Beacon) *proto.BeaconPacket {
	return &proto.BeaconPacket{
		PreviousSig: b.PreviousSig,
		Round:       b.Round,
		Signature:   b.Signature,
	}
}

func protoToBeacon(p *proto.BeaconPacket) *Beacon {
	return &Beacon{
		Round:       p.GetRound(),
		Signature:   p.GetSignature(),
		PreviousSig: p.GetPreviousSig(),
	}
}

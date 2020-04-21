package beacon

import (
	proto "github.com/drand/drand/protobuf/drand"
)

func beaconToProto(b *Beacon) *proto.BeaconPacket {
	return &proto.BeaconPacket{
		PreviousRound: b.PreviousRound,
		PreviousSig:   b.PreviousSig,
		Round:         b.Round,
		Signature:     b.Signature,
	}
}

func protoToBeacon(p *proto.BeaconPacket) *Beacon {
	return &Beacon{
		Round:         p.GetRound(),
		PreviousRound: p.GetPreviousRound(),
		Signature:     p.GetSignature(),
		PreviousSig:   p.GetPreviousSig(),
	}
}

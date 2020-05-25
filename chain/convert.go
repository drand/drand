package chain

import (
	"fmt"
	"io"
	"time"

	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
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

// InfoFromProto returns a Info from the protocol description
func InfoFromProto(p *proto.ChainInfoPacket) (*Info, error) {
	public := key.KeyGroup.Point()
	if err := public.UnmarshalBinary(p.PublicKey); err != nil {
		return nil, err
	}

	return &Info{
		PublicKey:   public,
		GenesisTime: p.GenesisTime,
		Period:      time.Duration(p.Period) * time.Second,
	}, nil
}

// ToProto returns the protobuf description of the chain info
func (c *Info) ToProto() *drand.ChainInfoPacket {
	buff, _ := c.PublicKey.MarshalBinary()
	return &drand.ChainInfoPacket{
		PublicKey:   buff,
		GenesisTime: c.GenesisTime,
		Period:      uint32(c.Period.Seconds()),
	}
}

// InfoFromJSON returns a Info from JSON description in the given reader
func InfoFromJSON(buff io.Reader) (*Info, error) {
	chainProto := new(drand.ChainInfoPacket)
	if err := json.NewDecoder(buff).Decode(chainProto); err != nil {
		return nil, fmt.Errorf("reading group file (%v)", err)
	}
	chainInfo, err := InfoFromProto(chainProto)
	if err != nil {
		return nil, fmt.Errorf("invalid chain info: %s", err)
	}
	return chainInfo, nil
}

func (c *Info) ToJSON(w io.Writer) error {
	proto := c.ToProto()
	return json.NewEncoder(w).Encode(proto)
}

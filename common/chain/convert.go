package chain

import (
	"fmt"
	"io"
	"time"

	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/common/crypto"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
)

// InfoFromProto returns a Info from the protocol description
func InfoFromProto(p *drand.ChainInfoPacket) (*Info, error) {
	sch, err := crypto.GetSchemeByIDWithDefault(p.SchemeID)
	if err != nil {
		return nil, fmt.Errorf("scheme id received is not valid. Err: %w", err)
	}
	public := sch.KeyGroup.Point()
	if err := public.UnmarshalBinary(p.PublicKey); err != nil {
		return nil, err
	}

	return &Info{
		PublicKey:   public,
		GenesisTime: p.GenesisTime,
		Period:      time.Duration(p.Period) * time.Second,
		GenesisSeed: p.GroupHash,
		Scheme:      sch.Name,
		ID:          p.GetMetadata().GetBeaconID(),
	}, nil
}

// ToProto returns the protobuf description of the chain info
func (c *Info) ToProto(metadata *common.Metadata) *drand.ChainInfoPacket {
	buff, _ := c.PublicKey.MarshalBinary()

	if metadata != nil {
		metadata.BeaconID = c.ID
	} else {
		metadata = &common.Metadata{BeaconID: c.ID}
	}

	return &drand.ChainInfoPacket{
		PublicKey:   buff,
		GenesisTime: c.GenesisTime,
		Period:      uint32(c.Period.Seconds()),
		Hash:        c.Hash(),
		GroupHash:   c.GenesisSeed,
		SchemeID:    c.Scheme,
		Metadata:    metadata,
	}
}

// InfoFromJSON returns a Info from JSON description in the given reader
func InfoFromJSON(buff io.Reader) (*Info, error) {
	chainProto := new(drand.ChainInfoPacket)
	if err := json.NewDecoder(buff).Decode(chainProto); err != nil {
		return nil, fmt.Errorf("reading group file (%w)", err)
	}

	chainInfo, err := InfoFromProto(chainProto)
	if err != nil {
		return nil, fmt.Errorf("invalid chain info: %w", err)
	}

	return chainInfo, nil
}

// ToJSON provides a json serialization of an info packet
func (c *Info) ToJSON(w io.Writer, metadata *common.Metadata) error {
	info := c.ToProto(metadata)
	return json.NewEncoder(w).Encode(info)
}

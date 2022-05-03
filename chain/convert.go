package chain

import (
	"fmt"
	"io"
	"time"

	commonutils "github.com/drand/drand/common"

	"github.com/drand/drand/protobuf/common"

	"github.com/drand/drand/common/scheme"

	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
)

// InfoFromProto returns a Info from the protocol description
func InfoFromProto(p *drand.ChainInfoPacket) (*Info, error) {
	public := key.KeyGroup.Point()
	if err := public.UnmarshalBinary(p.PublicKey); err != nil {
		return nil, err
	}

	sch, err := scheme.GetSchemeByIDWithDefault(p.SchemeID)
	if err != nil {
		return nil, fmt.Errorf("scheme id received is not valid. Err: %w", err)
	}

	return &Info{
		PublicKey:   public,
		GenesisTime: p.GenesisTime,
		Period:      time.Duration(p.Period) * time.Second,
		GroupHash:   p.GroupHash,
		Scheme:      sch,
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

	if metadata.BeaconID == "" {
		metadata.BeaconID = commonutils.DefaultBeaconID
	}

	return &drand.ChainInfoPacket{
		PublicKey:   buff,
		GenesisTime: c.GenesisTime,
		Period:      uint32(c.Period.Seconds()),
		Hash:        c.Hash(),
		GroupHash:   c.GroupHash,
		SchemeID:    c.Scheme.ID,
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

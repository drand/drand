package chain

import (
	"fmt"
	"io"
	"time"

	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/v2/protobuf/drand"

	"github.com/drand/drand/v2/crypto"
)

// InfoFromProto returns a Info from the protocol description
func InfoFromProto(p *drand.ChainInfoPacket) (*Info, error) {
	sch, err := crypto.GetSchemeByID(p.SchemeID)
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
func (i *Info) ToProto(metadata *drand.Metadata) *drand.ChainInfoPacket {
	buff, _ := i.PublicKey.MarshalBinary()

	if metadata != nil {
		metadata.BeaconID = i.ID
	} else {
		metadata = &drand.Metadata{BeaconID: i.ID}
	}

	return &drand.ChainInfoPacket{
		PublicKey:   buff,
		GenesisTime: i.GenesisTime,
		Period:      uint32(i.Period.Seconds()),
		Hash:        i.Hash(),
		GroupHash:   i.GenesisSeed,
		SchemeID:    i.Scheme,
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
func (i *Info) ToJSON(w io.Writer, metadata *drand.Metadata) error {
	info := i.ToProto(metadata)
	// Marshal proto to JSON first (ensures existing fields/encodings are preserved)
	b, err := json.Marshal(info)
	if err != nil {
		return err
	}
	// Unmarshal into a generic map to inject extra fields
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	m["catchup_period_seconds"] = uint32(i.CatchupPeriod.Seconds())
	return json.NewEncoder(w).Encode(m)
}

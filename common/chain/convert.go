package chain

import (
	"fmt"
	"io"
	"time"

	json "github.com/nikkolasg/hexjson"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

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

	periodProto := p.GetPeriod()
	if periodProto == nil || !periodProto.IsValid() {
		return nil, fmt.Errorf("invalid or missing period duration in ChainInfoPacket")
	}
	period := periodProto.AsDuration()
	if period <= 0 {
		return nil, fmt.Errorf("period must be positive in ChainInfoPacket")
	}

	genesisTimeProto := p.GetGenesisTime()
	if genesisTimeProto == nil || !genesisTimeProto.IsValid() {
		return nil, fmt.Errorf("invalid or missing genesis time in ChainInfoPacket")
	}
	genesisTime := genesisTimeProto.AsTime().Unix()

	return &Info{
		PublicKey:   public,
		GenesisTime: genesisTime,
		Period:      period,
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
		GenesisTime: timestamppb.New(time.Unix(i.GenesisTime, 0)),
		Period:      durationpb.New(i.Period),
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
	return json.NewEncoder(w).Encode(info)
}

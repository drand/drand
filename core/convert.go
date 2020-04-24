package core

import (
	"fmt"
	"net"
	"time"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	proto "github.com/drand/drand/protobuf/drand"
)

func ProtoToGroup(g *proto.GroupPacket) (*key.Group, error) {
	var nodes = make([]*key.Identity, 0, len(g.GetNodes()))
	for _, id := range g.GetNodes() {
		kid, err := protoToIdentity(id)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, kid)
	}
	n := len(nodes)
	thr := int(g.GetThreshold())
	if thr < key.MinimumT(n) {
		return nil, fmt.Errorf("invalid threshold: %d vs %d (minimum)", thr, key.MinimumT(n))
	}
	genesisTime := int64(g.GetGenesisTime())
	if genesisTime == 0 {
		return nil, fmt.Errorf("genesis time zero")
	}
	period := time.Duration(g.GetPeriod()) * time.Second
	if period == time.Duration(0) {
		return nil, fmt.Errorf("period time is zero")
	}
	var dist = new(key.DistPublic)
	for _, coeff := range g.DistKey {
		c := key.KeyGroup.Point()
		if err := c.UnmarshalBinary(coeff); err != nil {
			return nil, fmt.Errorf("invalid distributed key coefficients:%v", err)
		}
		dist.Coefficients = append(dist.Coefficients, c)
	}
	group := key.NewGroup(nodes, thr, genesisTime)
	// XXX Change the group creation methods to avoid this
	group.Period = period
	group.TransitionTime = int64(g.GetTransitionTime())
	if g.GetGenesisSeed() != nil {
		group.GenesisSeed = g.GetGenesisSeed()
	}
	if len(dist.Coefficients) > 0 {
		group.PublicKey = dist
	}
	return group, nil
}

func groupToProto(g *key.Group) *proto.GroupPacket {
	var out = new(proto.GroupPacket)
	var ids = make([]*proto.Identity, len(g.Nodes))
	for i, id := range g.Nodes {
		key, _ := id.Key.MarshalBinary()
		ids[i] = &proto.Identity{
			Address: id.Address(),
			Tls:     id.IsTLS(),
			Key:     key,
		}
	}
	out.Nodes = ids
	out.Period = uint32(g.Period.Seconds())
	out.Threshold = uint32(g.Threshold)
	out.GenesisTime = uint64(g.GenesisTime)
	out.TransitionTime = uint64(g.TransitionTime)
	out.GenesisSeed = g.GetGenesisSeed()
	if g.PublicKey != nil {
		var coeffs = make([][]byte, len(g.PublicKey.Coefficients))
		for i, c := range g.PublicKey.Coefficients {
			buff, _ := c.MarshalBinary()
			coeffs[i] = buff
		}
		out.DistKey = coeffs
	}
	return out
}

func protoToIdentity(n *proto.Identity) (*key.Identity, error) {
	_, _, err := net.SplitHostPort(n.GetAddress())
	if err != nil {
		return nil, err
	}
	public := key.KeyGroup.Point()
	if err := public.UnmarshalBinary(n.GetKey()); err != nil {
		return nil, err
	}
	return &key.Identity{
		Addr: n.GetAddress(),
		TLS:  n.Tls,
		Key:  public,
	}, nil
}

func beaconToProto(b *beacon.Beacon) *drand.PublicRandResponse {
	return &drand.PublicRandResponse{
		Round:             b.Round,
		Signature:         b.Signature,
		PreviousSignature: b.PreviousSig,
		Randomness:        b.Randomness(),
	}
}

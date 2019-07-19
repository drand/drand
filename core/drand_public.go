package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dedis/drand/beacon"
	"github.com/dedis/drand/ecies"
	"github.com/dedis/drand/entropy"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/protobuf/crypto"
	dkg_proto "github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
	"go.dedis.ch/kyber/v3"
	"google.golang.org/grpc/peer"
)

// Setup is the public method to call during a DKG protocol.
func (d *Drand) Setup(c context.Context, in *dkg_proto.DKGPacket) (*dkg_proto.DKGResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.dkgDone {
		return nil, errors.New("drand: dkg finished already")
	}
	if d.dkg == nil {
		return nil, errors.New("drand: no dkg running")
	}
	d.dkg.Process(c, in)
	return &dkg_proto.DKGResponse{}, nil
}

// Reshare is called when a resharing protocol is in progress
func (d *Drand) Reshare(c context.Context, in *dkg_proto.ResharePacket) (*dkg_proto.ReshareResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()

	if d.nextGroupHash == "" {
		return nil, fmt.Errorf("drand %s: can't reshare because InitReshare has not been called", d.priv.Public.Addr)
	}

	// check that we are resharing to the new group that we expect
	if in.GroupHash != d.nextGroupHash {
		return nil, errors.New("drand: can't reshare to new group: incompatible hashes")
	}

	if !d.nextFirstReceived && d.nextOldPresent {
		d.nextFirstReceived = true
		// go routine since StartDKG requires the global lock
		go d.StartDKG()
	}

	if d.dkg == nil {
		return nil, errors.New("drand: no dkg setup yet")
	}

	d.nextFirstReceived = true
	if in.Packet != nil {
		// first packet from the "leader" contains a nil packet for
		// nodes that are in the old list that must broadcast their
		// deals.
		d.dkg.Process(c, in.Packet)
	}
	return &dkg_proto.ReshareResponse{}, nil
}

// NewBeacon methods receives a beacon generation requests and answers
// with the partial signature from this drand node.
func (d *Drand) NewBeacon(c context.Context, in *drand.BeaconRequest) (*drand.BeaconResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.beacon == nil {
		return nil, errors.New("drand: beacon not setup yet")
	}
	return d.beacon.ProcessBeacon(c, in)
}

// Public returns a public random beacon according to the request. If the Round
// field is 0, then it returns the last one generated.
func (d *Drand) Public(c context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.beacon == nil {
		return nil, errors.New("drand: beacon generation not started yet")
	}
	var beacon *beacon.Beacon
	var err error
	if in.GetRound() == 0 {
		beacon, err = d.beaconStore.Last()
	} else {
		beacon, err = d.beaconStore.Get(in.GetRound())
	}
	if err != nil {
		return nil, fmt.Errorf("can't retrieve beacon: %s", err)
	}

	return &drand.PublicRandResponse{
		Previous: beacon.PreviousSig,
		Round:    beacon.Round,
		Signature: &crypto.Point{
			Point: beacon.Signature,
			Gid:   crypto.GroupID(beacon.Gid),
		},
		Randomness: beacon.Randomness,
	}, nil
}

// Private returns an ECIES encrypted random blob of 32 bytes from /dev/urandom
func (d *Drand) Private(c context.Context, priv *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	protoPoint := priv.GetRequest().GetEphemeral()
	point, err := crypto.ProtoToKyberPoint(protoPoint)
	if err != nil {
		return nil, err
	}
	groupable, ok := point.(kyber.Groupable)
	if !ok {
		return nil, errors.New("point is not on a registered curve")
	}
	if groupable.Group().String() != key.G2.String() {
		return nil, errors.New("point is not on the supported curve")
	}
	msg, err := ecies.Decrypt(key.G2, ecies.DefaultHash, d.priv.Key, priv.GetRequest())
	if err != nil {
		d.log.With("module", "public").Error("private", "invalid ECIES", "err", err.Error())
		return nil, errors.New("invalid ECIES request")
	}

	clientKey := key.G2.Point()
	if err := clientKey.UnmarshalBinary(msg); err != nil {
		return nil, errors.New("invalid client key")
	}
	randomness, err := entropy.GetRandom(nil, 32)
	if err != nil {
		return nil, fmt.Errorf("error gathering randomness: %s", err)
	} else if len(randomness) != 32 {
		return nil, fmt.Errorf("error gathering randomness: expected 32 bytes, got %d", len(randomness))
	}

	obj, err := ecies.Encrypt(key.G2, ecies.DefaultHash, clientKey, randomness[:])
	return &drand.PrivateRandResponse{Response: obj}, err
}

// Home ...
func (d *Drand) Home(c context.Context, in *drand.HomeRequest) (*drand.HomeResponse, error) {
	peer, _ := peer.FromContext(c)
	d.log.With("module", "public").Info("home", peer.Addr.String())
	return &drand.HomeResponse{
		Status: fmt.Sprintf("drand up and running on %s",
			d.priv.Public.Address()),
	}, nil
}

// Group replies with the current group of this drand node in a TOML encoded
// format
func (d *Drand) Group(ctx context.Context, in *drand.GroupRequest) (*drand.GroupResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.group == nil {
		return nil, errors.New("drand: no dkg group setup yet")
	}
	gtoml := d.group.TOML().(*key.GroupTOML)
	var resp = new(drand.GroupResponse)
	resp.Nodes = make([]*drand.Node, len(gtoml.Nodes))
	for i, n := range gtoml.Nodes {
		resp.Nodes[i] = &drand.Node{
			Address: n.Address,
			Key:     n.Key,
			TLS:     n.TLS,
		}
	}
	resp.Threshold = uint32(gtoml.Threshold)
	// take the period in second -> ms. grouptoml already transforms it to toml
	ms := uint32(d.group.Period / time.Millisecond)
	resp.Period = ms
	if gtoml.PublicKey != nil {
		resp.Distkey = make([]string, len(gtoml.PublicKey.Coefficients))
		copy(resp.Distkey, gtoml.PublicKey.Coefficients)
	}
	return resp, nil
}

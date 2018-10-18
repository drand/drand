package core

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/dedis/drand/ecies"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/protobuf/crypto"
	dkg_proto "github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/kyber"
	"github.com/nikkolasg/slog"
)

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
		return nil, errors.New("drand: can't reshare because InitReshare has not been called")
	}

	// check that we are resharing to the new group that we expect
	if in.GroupHash != d.nextGroupHash {
		fmt.Println("in.GroupHash = > ", in.GroupHash, " vs d.nestGrouphash ", d.nextGroupHash)
		return nil, errors.New("drand: can't reshare to new group: incompatible hashes")
	}

	if in.Packet == nil {
		// indicator that we should start the DKG as we are one node in the old
		// list that should reshare its share
		go d.StartDKG()
		return &dkg_proto.ReshareResponse{}, nil
	}

	if d.dkg == nil {
		return nil, errors.New("drand: no dkg setup yet")
	}

	// we just relay to the dkg
	d.dkg.Process(c, in.Packet)
	return &dkg_proto.ReshareResponse{}, nil
}

func (d *Drand) NewBeacon(c context.Context, in *drand.BeaconRequest) (*drand.BeaconResponse, error) {
	if !d.isDKGDone() {
		return nil, errors.New("drand: dkg not finished")
	}
	if d.beacon == nil {
		panic("that's not ever should happen so I'm panicking right now")
	}
	return d.beacon.ProcessBeacon(c, in)
}
func (d *Drand) Public(context.Context, *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	beacon, err := d.beaconStore.Last()
	if err != nil {
		return nil, fmt.Errorf("can't retrieve beacon: %s", err)
	}
	return &drand.PublicRandResponse{
		Previous:   beacon.PreviousRand,
		Round:      beacon.Round,
		Randomness: beacon.Randomness,
	}, nil
}

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
		slog.Debugf("drand: received invalid ECIES private request: %s", err)
		return nil, errors.New("invalid ECIES request")
	}

	clientKey := key.G2.Point()
	if err := clientKey.UnmarshalBinary(msg); err != nil {
		return nil, errors.New("invalid client key")
	}
	var randomness [32]byte
	if n, err := rand.Read(randomness[:]); err != nil {
		return nil, errors.New("error gathering randomness")
	} else if n != 32 {
		return nil, errors.New("error gathering randomness")
	}

	obj, err := ecies.Encrypt(key.G2, ecies.DefaultHash, clientKey, randomness[:])
	return &drand.PrivateRandResponse{Response: obj}, err
}

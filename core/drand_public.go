package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/ecies"
	"github.com/drand/drand/entropy"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/grpc/peer"
)

// Setup is the public method to call during a DKG protocol.
func (d *Drand) FreshDKG(c context.Context, in *drand.DKGPacket) (*drand.Empty, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.dkgDone {
		return nil, errors.New("drand: dkg finished already")
	}
	if d.dkg == nil {
		return nil, errors.New("drand: no dkg running")
	}
	d.dkg.Process(c, in.Dkg)
	return new(drand.Empty), nil
}

// Reshare is called when a resharing protocol is in progress
func (d *Drand) ReshareDKG(c context.Context, in *drand.ResharePacket) (*drand.Empty, error) {
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
		go d.StartDKG(d.nextConf)
	}

	if d.dkg == nil {
		return nil, errors.New("drand: no dkg setup yet")
	}

	d.nextFirstReceived = true
	if in.Dkg != nil {
		// first packet from the "leader" contains a nil packet for
		// nodes that are in the old list that must broadcast their
		// deals.
		d.dkg.Process(c, in.Dkg)
	}
	return new(drand.Empty), nil
}

// NewBeacon methods receives a beacon generation requests and answers
// with the partial signature from this drand node.
func (d *Drand) NewBeacon(c context.Context, in *drand.BeaconPacket) (*drand.Empty, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.beacon == nil {
		return nil, errors.New("drand: beacon not setup yet")
	}
	return d.beacon.ProcessBeacon(c, in)
}

// PublicRand returns a public random beacon according to the request. If the Round
// field is 0, then it returns the last one generated.
func (d *Drand) PublicRand(c context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.beacon == nil {
		return nil, errors.New("drand: beacon generation not started yet")
	}
	var beacon *beacon.Beacon
	var err error
	if in.GetRound() == 0 {
		beacon, err = d.beacon.Store().Last()
	} else {
		beacon, err = d.beacon.Store().Get(in.GetRound())
	}
	if err != nil {
		return nil, fmt.Errorf("can't retrieve beacon: %s", err)
	}
	peer, ok := peer.FromContext(c)
	if ok {
		d.log.With("module", "public").Info("public_rand", peer.Addr.String(), "round", beacon.Round)
		d.log.Info("public rand", peer.Addr.String(), "round", beacon.Round)
	}
	return &drand.PublicRandResponse{
		PreviousSignature: beacon.PreviousSig,
		PreviousRound:     beacon.PreviousRound,
		Round:             beacon.Round,
		Signature:         beacon.Signature,
		Randomness:        beacon.Randomness(),
	}, nil
}

func (d *Drand) PublicRandStream(req *drand.PublicRandRequest, stream drand.Public_PublicRandStreamServer) error {
	var b *beacon.Handler
	d.state.Lock()
	if d.beacon == nil {
		return errors.New("beacon has not started on this node yet")
	}
	b = d.beacon
	d.state.Unlock()
	lastb, err := b.Store().Last()
	if err != nil {
		return err
	}
	peer, _ := peer.FromContext(stream.Context())
	addr := peer.Addr.String()
	done := make(chan error, 1)
	d.log.Debug("request", "stream", "from", addr, "round", req.GetRound())
	if req.GetRound() <= lastb.Round {
		// we need to stream from store first
		var err error
		b.Store().Cursor(func(c beacon.Cursor) {
			for bb := c.Seek(req.GetRound()); bb != nil; bb = c.Next() {
				if err = stream.Send(beaconToProto(bb)); err != nil {
					d.log.Debug("stream", err)
					return
				}
			}
		})
		if err != nil {
			return err
		}
	}
	// then we can stream from any new rounds
	// register a callback for the duration of this stream
	d.callbacks.AddCallback(addr, func(b *beacon.Beacon) {
		err := stream.Send(&drand.PublicRandResponse{
			Round:             b.Round,
			Signature:         b.Signature,
			PreviousRound:     b.PreviousRound,
			PreviousSignature: b.PreviousSig,
			Randomness:        b.Randomness(),
		})
		// if connection has a problem, we drop the callback
		if err != nil {
			d.callbacks.DelCallback(addr)
			done <- err
		}
	})
	return <-done
}

// PrivateRand returns an ECIES encrypted random blob of 32 bytes from /dev/urandom
func (d *Drand) PrivateRand(c context.Context, priv *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	protoPoint := priv.GetRequest().GetEphemeral()
	point := key.KeyGroup.Point()
	if err := point.UnmarshalBinary(protoPoint); err != nil {
		return nil, err
	}
	msg, err := ecies.Decrypt(key.KeyGroup, ecies.DefaultHash, d.priv.Key, priv.GetRequest())
	if err != nil {
		d.log.With("module", "public").Error("private", "invalid ECIES", "err", err.Error())
		return nil, errors.New("invalid ECIES request")
	}

	clientKey := key.KeyGroup.Point()
	if err := clientKey.UnmarshalBinary(msg); err != nil {
		return nil, errors.New("invalid client key")
	}
	randomness, err := entropy.GetRandom(nil, 32)
	if err != nil {
		return nil, fmt.Errorf("error gathering randomness: %s", err)
	} else if len(randomness) != 32 {
		return nil, fmt.Errorf("error gathering randomness: expected 32 bytes, got %d", len(randomness))
	}

	obj, err := ecies.Encrypt(key.KeyGroup, ecies.DefaultHash, clientKey, randomness[:])
	return &drand.PrivateRandResponse{Response: obj}, err
}

// Home ...
func (d *Drand) Home(c context.Context, in *drand.HomeRequest) (*drand.HomeResponse, error) {
	peer, ok := peer.FromContext(c)
	if ok {
		d.log.With("module", "public").Info("home", peer.Addr.String())
	}
	return &drand.HomeResponse{
		Status: fmt.Sprintf("drand up and running on %s",
			d.priv.Public.Address()),
	}, nil
}

// Group replies with the current group of this drand node in a TOML encoded
// format
func (d *Drand) Group(ctx context.Context, in *drand.GroupRequest) (*drand.GroupPacket, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.group == nil {
		return nil, errors.New("drand: no dkg group setup yet")
	}
	return groupToProto(d.group), nil
}

func (d *Drand) PrepareDKGGroup(ctx context.Context, p *drand.PrepareDKGPacket) (*drand.GroupPacket, error) {
	var receivers *groupReceiver
	verif := func() error {
		d.state.Lock()
		defer d.state.Unlock()
		if d.manager == nil {
			return errors.New("no manager")
		}
		peer, ok := peer.FromContext(ctx)
		if !ok {
			return errors.New("no peer associated")
		}
		// manager will verify if information are correct
		var err error
		receivers, err = d.manager.ReceivedKey(peer.Addr.String(), p)
		if err != nil {
			return err
		}
		return nil
	}
	if err := verif(); err != nil {
		return nil, err
	}
	defer func() { receivers.DoneCh <- true }()
	// wait for the group to be ready,i.e. all other participants sent their
	// keys as well. Channel is automatically close after a while.
	group := <-receivers.WaitGroup
	if group == nil {
		return nil, errors.New("no valid group has been generated in time")
	}
	// reply with the group, the receiver will start the DKG
	protoGroup := groupToProto(group)
	return protoGroup, nil
}

func beaconToProto(b *beacon.Beacon) *drand.PublicRandResponse {
	return &drand.PublicRandResponse{
		Round:             b.Round,
		Signature:         b.Signature,
		PreviousRound:     b.PreviousRound,
		PreviousSignature: b.PreviousSig,
		Randomness:        b.Randomness(),
	}
}

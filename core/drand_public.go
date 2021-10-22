package core

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/drand/drand/protobuf/common"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/beacon"
	"github.com/drand/drand/entropy"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/encrypt/ecies"
)

// BroadcastDKG is the public method to call during a DKG protocol.
func (d *Drand) BroadcastDKG(c context.Context, in *drand.DKGPacket) (*drand.Empty, error) {
	d.state.Lock()
	defer d.state.Unlock()

	if d.dkgInfo == nil {
		return nil, errors.New("drand: no dkg running")
	}
	addr := net.RemoteAddress(c)

	if !d.dkgInfo.started {
		d.log.Infow("", "beacon_id", d.dkgInfo.target.ID, "init_dkg", "START DKG",
			"signal from leader", addr, "group", hex.EncodeToString(d.dkgInfo.target.Hash()))
		d.dkgInfo.started = true
		go d.dkgInfo.phaser.Start()
	}
	if _, err := d.dkgInfo.board.BroadcastDKG(c, in); err != nil {
		return nil, err
	}

	response := &drand.Empty{Metadata: common.NewMetadata(d.version.ToProto())}
	return response, nil
}

// PartialBeacon receives a beacon generation request and answers
// with the partial signature from this drand node.
func (d *Drand) PartialBeacon(c context.Context, in *drand.PartialBeaconPacket) (*drand.Empty, error) {
	d.state.Lock()
	if d.beacon == nil {
		d.state.Unlock()
		return nil, errors.New("drand: beacon not setup yet")
	}
	inst := d.beacon
	d.state.Unlock()

	_, err := inst.ProcessPartialBeacon(c, in)
	return &drand.Empty{Metadata: common.NewMetadata(d.version.ToProto())}, err
}

// PublicRand returns a public random beacon according to the request. If the Round
// field is 0, then it returns the last one generated.
func (d *Drand) PublicRand(c context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	var addr = net.RemoteAddress(c)

	d.state.Lock()
	defer d.state.Unlock()

	if d.beacon == nil {
		return nil, errors.New("drand: beacon generation not started yet")
	}
	var beaconResp *chain.Beacon
	var err error
	if in.GetRound() == 0 {
		beaconResp, err = d.beacon.Store().Last()
	} else {
		// fetch the correct entry or the next one if not found
		beaconResp, err = d.beacon.Store().Get(in.GetRound())
	}
	if err != nil || beaconResp == nil {
		d.log.Debugw("", "public_rand", "unstored_beacon", "round", in.GetRound(), "from", addr)
		return nil, fmt.Errorf("can't retrieve beacon: %w %s", err, beaconResp)
	}
	d.log.Infow("", "public_rand", addr, "round", beaconResp.Round, "reply", beaconResp.String())

	response := beaconToProto(beaconResp)
	response.Metadata = common.NewMetadata(d.version.ToProto())

	return response, nil
}

// PublicRandStream exports a stream of new beacons as they are generated over gRPC
func (d *Drand) PublicRandStream(req *drand.PublicRandRequest, stream drand.Public_PublicRandStreamServer) error {
	var b *beacon.Handler

	d.state.Lock()
	if d.beacon == nil {
		d.state.Unlock()
		return errors.New("beacon has not started on this node yet")
	}
	b = d.beacon
	d.state.Unlock()

	lastb, err := b.Store().Last()
	if err != nil {
		return err
	}
	addr := net.RemoteAddress(stream.Context())
	done := make(chan error, 1)
	d.log.Debugw("", "request", "stream", "from", addr, "round", req.GetRound())
	if req.GetRound() != 0 && req.GetRound() <= lastb.Round {
		// we need to stream from store first
		var err error
		b.Store().Cursor(func(c chain.Cursor) {
			for bb := c.Seek(req.GetRound()); bb != nil; bb = c.Next() {
				if err = stream.Send(beaconToProto(bb)); err != nil {
					d.log.Debugw("", "stream", err)
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
	d.beacon.AddCallback(addr, func(b *chain.Beacon) {
		err := stream.Send(&drand.PublicRandResponse{
			Round:             b.Round,
			Signature:         b.Signature,
			PreviousSignature: b.PreviousSig,
			Randomness:        b.Randomness(),
			Metadata:          common.NewMetadata(d.version.ToProto()),
		})
		// if connection has a problem, we drop the callback
		if err != nil {
			d.beacon.RemoveCallback(addr)
			done <- err
		}
	})
	return <-done
}

// PrivateRand returns an ECIES encrypted random blob of 32 bytes from /dev/urandom
func (d *Drand) PrivateRand(c context.Context, priv *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	if !d.opts.enablePrivate {
		return nil, errors.New("private randomness is disabled")
	}
	msg, err := ecies.Decrypt(key.KeyGroup, d.priv.Key, priv.GetRequest(), EciesHash)
	if err != nil {
		d.log.With("module", "public").Errorw("", "private", "invalid ECIES", "err", err.Error())
		return nil, errors.New("invalid ECIES request")
	}

	clientKey := key.KeyGroup.Point()
	if err := clientKey.UnmarshalBinary(msg); err != nil {
		return nil, errors.New("invalid client key")
	}
	randomness, err := entropy.GetRandom(nil, PrivateRandLength)
	if err != nil {
		return nil, fmt.Errorf("error gathering randomness: %s", err)
	} else if len(randomness) != PrivateRandLength {
		return nil, fmt.Errorf("error gathering randomness: expected 32 bytes, got %d", len(randomness))
	}

	obj, err := ecies.Encrypt(key.KeyGroup, clientKey, randomness, EciesHash)

	return &drand.PrivateRandResponse{Response: obj, Metadata: common.NewMetadata(d.version.ToProto())}, err
}

// Home provides the address the local node is listening
func (d *Drand) Home(c context.Context, in *drand.HomeRequest) (*drand.HomeResponse, error) {
	d.log.With("module", "public").Infow("", "home", net.RemoteAddress(c))

	return &drand.HomeResponse{
		Status: fmt.Sprintf("drand up and running on %s",
			d.priv.Public.Address()),
		Metadata: common.NewMetadata(d.version.ToProto()),
	}, nil
}

// ChainInfo replies with the chain information this node participates to
func (d *Drand) ChainInfo(ctx context.Context, in *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.group == nil {
		return nil, errors.New("drand: no dkg group setup yet")
	}

	metadata := common.NewMetadata(d.version.ToProto())
	response := chain.NewChainInfo(d.group).ToProto(metadata)

	return response, nil
}

// SignalDKGParticipant receives a dkg signal packet from another member
func (d *Drand) SignalDKGParticipant(ctx context.Context, p *drand.SignalDKGPacket) (*drand.Empty, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.manager == nil {
		return nil, errors.New("no manager")
	}
	addr := net.RemoteAddress(ctx)
	// manager will verify if information are correct
	err := d.manager.ReceivedKey(addr, p)
	if err != nil {
		return nil, err
	}

	response := &drand.Empty{Metadata: common.NewMetadata(d.version.ToProto())}
	return response, nil
}

// PushDKGInfo triggers sending DKG info to other members
func (d *Drand) PushDKGInfo(ctx context.Context, in *drand.DKGInfoPacket) (*drand.Empty, error) {
	d.state.Lock()
	defer d.state.Unlock()

	beaconID := in.GetMetadata().GetBeaconID()
	if d.receiver == nil {
		return nil, errors.New("no receiver setup")
	}

	d.log.Infow("", "beacon_id", beaconID, "push_group", "received_new")

	// the control routine will receive this info and start the dkg at the right
	// time - if that is the right secret.
	response := &drand.Empty{Metadata: common.NewMetadata(d.version.ToProto())}
	return response, d.receiver.PushDKGInfo(in)
}

// SyncChain is a inter-node protocol that replies to a syncing request from a
// given round
func (d *Drand) SyncChain(req *drand.SyncRequest, stream drand.Protocol_SyncChainServer) error {
	d.state.Lock()
	b := d.beacon
	d.state.Unlock()

	if b != nil {
		return b.SyncChain(req, stream)
	}
	return nil
}

// GetIdentity returns the identity of this drand node
func (d *Drand) GetIdentity(ctx context.Context, req *drand.IdentityRequest) (*drand.IdentityResponse, error) {
	i := d.priv.Public.ToProto()
	metadata := common.NewMetadata(d.version.ToProto())

	response := &drand.IdentityResponse{
		Address:   i.Address,
		Key:       i.Key,
		Tls:       i.Tls,
		Signature: i.Signature,
		Metadata:  metadata,
	}
	return response, nil
}

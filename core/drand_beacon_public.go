package core

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/beacon"
	"github.com/drand/drand/entropy"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/encrypt/ecies"
)

// BroadcastDKG is the public method to call during a DKG protocol.
func (bp *BeaconProcess) BroadcastDKG(c context.Context, in *drand.DKGPacket) (*drand.Empty, error) {
	bp.state.Lock()
	defer bp.state.Unlock()

	if bp.dkgInfo == nil {
		return nil, errors.New("drand: no dkg running")
	}
	addr := net.RemoteAddress(c)

	if !bp.dkgInfo.started {
		bp.log.Infow("", "init_dkg", "START DKG",
			"signal from leader", addr, "group", hex.EncodeToString(bp.dkgInfo.target.Hash()))
		bp.dkgInfo.started = true
		go bp.dkgInfo.phaser.Start()
	}
	if _, err := bp.dkgInfo.board.BroadcastDKG(c, in); err != nil {
		return nil, err
	}

	response := &drand.Empty{Metadata: common.NewMetadata(bp.version.ToProto())}
	return response, nil
}

// PartialBeacon receives a beacon generation request and answers
// with the partial signature from this drand node.
func (bp *BeaconProcess) PartialBeacon(c context.Context, in *drand.PartialBeaconPacket) (*drand.Empty, error) {
	bp.state.Lock()
	if bp.beacon == nil {
		bp.state.Unlock()
		return nil, errors.New("drand: beacon not setup yet")
	}
	inst := bp.beacon
	bp.state.Unlock()

	_, err := inst.ProcessPartialBeacon(c, in)
	return &drand.Empty{Metadata: common.NewMetadata(bp.version.ToProto())}, err
}

// PublicRand returns a public random beacon according to the request. If the Round
// field is 0, then it returns the last one generated.
func (bp *BeaconProcess) PublicRand(c context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	var addr = net.RemoteAddress(c)

	bp.state.Lock()
	defer bp.state.Unlock()

	if bp.beacon == nil {
		return nil, errors.New("drand: beacon generation not started yet")
	}
	var beaconResp *chain.Beacon
	var err error
	if in.GetRound() == 0 {
		beaconResp, err = bp.beacon.Store().Last()
	} else {
		// fetch the correct entry or the next one if not found
		beaconResp, err = bp.beacon.Store().Get(in.GetRound())
	}
	if err != nil || beaconResp == nil {
		bp.log.Debugw("", "public_rand", "unstored_beacon", "round", in.GetRound(), "from", addr)
		return nil, fmt.Errorf("can't retrieve beacon: %w %s", err, beaconResp)
	}
	bp.log.Infow("", "public_rand", addr, "round", beaconResp.Round, "reply", beaconResp.String())

	response := beaconToProto(beaconResp)
	response.Metadata = common.NewMetadata(bp.version.ToProto())

	return response, nil
}

// PublicRandStream exports a stream of new beacons as they are generated over gRPC
func (bp *BeaconProcess) PublicRandStream(req *drand.PublicRandRequest, stream drand.Public_PublicRandStreamServer) error {
	var b *beacon.Handler

	bp.state.Lock()
	if bp.beacon == nil {
		bp.state.Unlock()
		return errors.New("beacon has not started on this node yet")
	}
	b = bp.beacon
	bp.state.Unlock()

	lastb, err := b.Store().Last()
	if err != nil {
		return err
	}
	addr := net.RemoteAddress(stream.Context())
	done := make(chan error, 1)
	bp.log.Debugw("", "request", "stream", "from", addr, "round", req.GetRound())
	if req.GetRound() != 0 && req.GetRound() <= lastb.Round {
		// we need to stream from store first
		var err error
		logger := bp.log.Named("StoreCursor")
		b.Store().Cursor(func(c chain.Cursor) {
			for bb := c.Seek(req.GetRound()); bb != nil; bb = c.Next() {
				if err = stream.Send(beaconToProto(bb)); err != nil {
					logger.Debugw("", "stream", err)
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
	bp.beacon.AddCallback(addr, func(b *chain.Beacon) {
		err := stream.Send(&drand.PublicRandResponse{
			Round:             b.Round,
			Signature:         b.Signature,
			PreviousSignature: b.PreviousSig,
			Randomness:        b.Randomness(),
			Metadata:          common.NewMetadata(bp.version.ToProto()),
		})
		// if connection has a problem, we drop the callback
		if err != nil {
			bp.beacon.RemoveCallback(addr)
			done <- err
		}
	})
	return <-done
}

// PrivateRand returns an ECIES encrypted random blob of 32 bytes from /dev/urandom
func (bp *BeaconProcess) PrivateRand(c context.Context, priv *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	if !bp.opts.enablePrivate {
		return nil, errors.New("private randomness is disabled")
	}
	msg, err := ecies.Decrypt(key.KeyGroup, bp.priv.Key, priv.GetRequest(), EciesHash)
	if err != nil {
		bp.log.With("module", "public").Errorw("", "private", "invalid ECIES", "err", err.Error())
		return nil, errors.New("invalid ECIES request")
	}

	clientKey := key.KeyGroup.Point()
	if err := clientKey.UnmarshalBinary(msg); err != nil {
		return nil, errors.New("invalid client key")
	}
	randomness, err := entropy.GetRandom(nil, PrivateRandLength)
	if err != nil {
		return nil, fmt.Errorf("error gathering randomness: %w", err)
	} else if len(randomness) != PrivateRandLength {
		return nil, fmt.Errorf("error gathering randomness: expected 32 bytes, got %bp", len(randomness))
	}

	obj, err := ecies.Encrypt(key.KeyGroup, clientKey, randomness, EciesHash)

	return &drand.PrivateRandResponse{Response: obj, Metadata: common.NewMetadata(bp.version.ToProto())}, err
}

// Home provides the address the local node is listening
func (bp *BeaconProcess) Home(c context.Context, in *drand.HomeRequest) (*drand.HomeResponse, error) {
	bp.log.With("module", "public").Infow("", "home", net.RemoteAddress(c))

	return &drand.HomeResponse{
		Status: fmt.Sprintf("drand up and running on %s",
			bp.priv.Public.Address()),
		Metadata: common.NewMetadata(bp.version.ToProto()),
	}, nil
}

// ChainInfo replies with the chain information this node participates to
func (bp *BeaconProcess) ChainInfo(ctx context.Context, in *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	bp.state.Lock()
	defer bp.state.Unlock()
	if bp.group == nil {
		return nil, errors.New("drand: no dkg group setup yet")
	}

	metadata := common.NewMetadata(bp.version.ToProto())
	response := chain.NewChainInfo(bp.group).ToProto(metadata)

	return response, nil
}

// SignalDKGParticipant receives a dkg signal packet from another member
func (bp *BeaconProcess) SignalDKGParticipant(ctx context.Context, p *drand.SignalDKGPacket) (*drand.Empty, error) {
	bp.state.Lock()
	defer bp.state.Unlock()
	if bp.manager == nil {
		return nil, errors.New("no manager")
	}
	addr := net.RemoteAddress(ctx)
	// manager will verify if information are correct
	err := bp.manager.ReceivedKey(addr, p)
	if err != nil {
		return nil, err
	}

	response := &drand.Empty{Metadata: common.NewMetadata(bp.version.ToProto())}
	return response, nil
}

// PushDKGInfo triggers sending DKG info to other members
func (bp *BeaconProcess) PushDKGInfo(ctx context.Context, in *drand.DKGInfoPacket) (*drand.Empty, error) {
	bp.state.Lock()
	defer bp.state.Unlock()

	if bp.receiver == nil {
		return nil, errors.New("no receiver setup")
	}
	bp.log.Infow("", "push_group", "received_new")

	// the control routine will receive this info and start the dkg at the right
	// time - if that is the right secret.
	response := &drand.Empty{Metadata: common.NewMetadata(bp.version.ToProto())}
	return response, bp.receiver.PushDKGInfo(in)
}

// SyncChain is a inter-node protocol that replies to a syncing request from a
// given round
func (bp *BeaconProcess) SyncChain(req *drand.SyncRequest, stream drand.Protocol_SyncChainServer) error {
	bp.state.Lock()
	b := bp.beacon
	bp.state.Unlock()

	if b != nil {
		return b.SyncChain(req, stream)
	}
	return nil
}

// GetIdentity returns the identity of this drand node
func (bp *BeaconProcess) GetIdentity(ctx context.Context, req *drand.IdentityRequest) (*drand.IdentityResponse, error) {
	i := bp.priv.Public.ToProto()
	metadata := common.NewMetadata(bp.version.ToProto())

	response := &drand.IdentityResponse{
		Address:   i.Address,
		Key:       i.Key,
		Tls:       i.Tls,
		Signature: i.Signature,
		Metadata:  metadata,
	}
	return response, nil
}

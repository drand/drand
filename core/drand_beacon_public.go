package core

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/beacon"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
)

// BroadcastDKG is the public method to call during a DKG protocol.
func (bp *BeaconProcess) BroadcastDKG(c context.Context, in *drand.DKGPacket) (*drand.Empty, error) {
	bp.state.Lock()
	defer bp.state.Unlock()

	addr := net.RemoteAddress(c)

	if bp.dkgInfo == nil {
		return nil, fmt.Errorf("drand: no dkg running and yet received a DKGPacket for beacon %s from node %s", bp.beaconID, addr)
	}

	if !bp.dkgInfo.started {
		bp.log.Infow("", "init_dkg", "START DKG",
			"signal from leader", addr, "group", hex.EncodeToString(bp.dkgInfo.target.Hash()))
		bp.dkgInfo.started = true
		go bp.dkgInfo.phaser.Start()
	}
	if err := bp.dkgInfo.board.BroadcastDKG(c, in); err != nil {
		return nil, err
	}

	response := &drand.Empty{Metadata: bp.newMetadata()}
	return response, nil
}

// PartialBeacon receives a beacon generation request and answers
// with the partial signature from this drand node.
func (bp *BeaconProcess) PartialBeacon(c context.Context, in *drand.PartialBeaconPacket) (*drand.Empty, error) {
	bp.state.Lock()
	// we need to defer unlock here to avoid races during the partial processing
	defer bp.state.Unlock()
	inst := bp.beacon
	if inst == nil || len(bp.chainHash) == 0 {
		return nil, errors.New("DKG not finished yet")
	}

	_, err := inst.ProcessPartialBeacon(c, in)
	return &drand.Empty{Metadata: bp.newMetadata()}, err
}

// PublicRand returns a public random beacon according to the request. If the Round
// field is 0, then it returns the last one generated.
func (bp *BeaconProcess) PublicRand(ctx context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	var addr = net.RemoteAddress(ctx)

	bp.state.Lock()
	defer bp.state.Unlock()

	if bp.beacon == nil || len(bp.chainHash) == 0 {
		return nil, errors.New("drand: beacon generation not started yet")
	}
	var beaconResp *chain.Beacon
	var err error
	if in.GetRound() == 0 {
		beaconResp, err = bp.beacon.Store().Last(ctx)
	} else {
		// fetch the correct entry or the next one if not found
		beaconResp, err = bp.beacon.Store().Get(ctx, in.GetRound())
	}
	if err != nil || beaconResp == nil {
		bp.log.Debugw("", "public_rand", "unstored_beacon", "round", in.GetRound(), "from", addr)
		return nil, fmt.Errorf("can't retrieve beacon: %w %s", err, beaconResp)
	}
	bp.log.Debugw("", "public_rand", addr, "round", beaconResp.Round, "reply", beaconResp.String())

	response := beaconToProto(beaconResp)
	response.Metadata = bp.newMetadata()

	return response, nil
}

// a proxy type so public streaming request can use the same logic as in priate
// / protocol syncing request, even though the types differ, so it prevents
// changing the protobuf structs.
type proxyRequest struct {
	*drand.PublicRandRequest
}

func (p *proxyRequest) GetFromRound() uint64 {
	return p.PublicRandRequest.GetRound()
}

type proxyStream struct {
	drand.Public_PublicRandStreamServer
}

func (p *proxyStream) Send(b *drand.BeaconPacket) error {
	return p.Public_PublicRandStreamServer.Send(&drand.PublicRandResponse{
		Round:             b.Round,
		Signature:         b.Signature,
		PreviousSignature: b.PreviousSig,
		Randomness:        chain.RandomnessFromSignature(b.Signature),
		Metadata:          b.Metadata,
	})
}

// PublicRandStream exports a stream of new beacons as they are generated over gRPC
func (bp *BeaconProcess) PublicRandStream(req *drand.PublicRandRequest, stream drand.Public_PublicRandStreamServer) error {
	bp.state.Lock()
	if bp.beacon == nil || len(bp.chainHash) == 0 {
		bp.state.Unlock()
		return errors.New("beacon has not started on this node yet")
	}
	store := bp.beacon.Store()

	proxyReq := &proxyRequest{
		req,
	}
	// make sure we have the correct metadata
	proxyReq.Metadata = bp.newMetadata()
	proxyStr := &proxyStream{stream}
	bp.state.Unlock()
	return beacon.SyncChain(bp.log.Named("PublicRand"), store, proxyReq, proxyStr)
}

// Home provides the address the local node is listening
func (bp *BeaconProcess) Home(c context.Context, _ *drand.HomeRequest) (*drand.HomeResponse, error) {
	bp.log.With("module", "public").Infow("", "home", net.RemoteAddress(c))

	return &drand.HomeResponse{
		Status: fmt.Sprintf("drand up and running on %s",
			bp.priv.Public.Address()),
		Metadata: bp.newMetadata(),
	}, nil
}

// ChainInfo replies with the chain information this node participates to
func (bp *BeaconProcess) ChainInfo(context.Context, *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	bp.state.Lock()
	group := bp.group
	chainHash := bp.chainHash
	bp.state.Unlock()
	if group == nil || len(chainHash) == 0 {
		return nil, errors.New("no dkg group setup yet")
	}

	response := chain.NewChainInfo(group).ToProto(bp.newMetadata())

	return response, nil
}

// SignalDKGParticipant receives a dkg signal packet from another member
func (bp *BeaconProcess) SignalDKGParticipant(ctx context.Context, p *drand.SignalDKGPacket) (*drand.Empty, error) {
	bp.state.Lock()
	defer bp.state.Unlock()
	if bp.manager == nil {
		bp.log.Errorw("Unable to process incoming SignalDKGPacket, no DKG in progress", "target beacon", p.GetMetadata().GetBeaconID())
		return nil, fmt.Errorf("no DKG in progress for beaconID %s", p.GetMetadata().GetBeaconID())
	}

	addr := net.RemoteAddress(ctx)
	// manager will verify if information are correct
	err := bp.manager.ReceivedKey(addr, p)
	if err != nil {
		bp.log.Errorw("Unable to process incoming SignalDKGPacket", "error", err)
		return nil, err
	}

	response := &drand.Empty{Metadata: bp.newMetadata()}
	return response, nil
}

// PushDKGInfo triggers sending DKG info to other members
func (bp *BeaconProcess) PushDKGInfo(_ context.Context, in *drand.DKGInfoPacket) (*drand.Empty, error) {
	bp.state.Lock()
	defer bp.state.Unlock()

	if bp.receiver == nil {
		return nil, errors.New("no receiver setup")
	}
	bp.log.Infow("", "push_group", "received_new")

	// the control routine will receive this info and start the dkg at the right
	// time - if that is the right secret.
	response := &drand.Empty{Metadata: bp.newMetadata()}
	return response, bp.receiver.PushDKGInfo(in)
}

// SyncChain is an inter-node protocol that replies to a syncing request from a
// given round
func (bp *BeaconProcess) SyncChain(req *drand.SyncRequest, stream drand.Protocol_SyncChainServer) error {
	bp.state.Lock()
	b := bp.beacon
	c := bp.chainHash
	bp.state.Unlock()
	if b == nil || len(c) == 0 {
		bp.log.Errorw("Received a SyncRequest, but no beacon handler is set yet", "request", req)
		return fmt.Errorf("no beacon handler available")
	}

	// TODO: consider re-running the SyncChain command if we get a ErrNoBeaconStored back as it could be a follow cmd
	return beacon.SyncChain(bp.log.Named("SyncChain"), bp.beacon.Store(), req, stream)
}

// GetIdentity returns the identity of this drand node
func (bp *BeaconProcess) GetIdentity(context.Context, *drand.IdentityRequest) (*drand.IdentityResponse, error) {
	i := bp.priv.Public.ToProto()

	response := &drand.IdentityResponse{
		Address:   i.Address,
		Key:       i.Key,
		Tls:       i.Tls,
		Signature: i.Signature,
		Metadata:  bp.newMetadata(),
	}
	return response, nil
}

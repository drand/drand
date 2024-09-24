package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/drand/drand/v2/common"
	chain2 "github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/tracer"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/chain/beacon"
	"github.com/drand/drand/v2/internal/net"
	"github.com/drand/drand/v2/protobuf/drand"
)

// PartialBeacon receives a beacon generation request and answers
// with the partial signature from this drand node.
func (bp *BeaconProcess) PartialBeacon(ctx context.Context, in *drand.PartialBeaconPacket) (*drand.Empty, error) {
	ctx, span := tracer.NewSpan(ctx, "bp.PartialBeacon")
	defer span.End()

	bp.state.RLock()
	// we need to defer unlock here to avoid races during the partial processing
	defer bp.state.RUnlock()
	inst := bp.beacon
	if inst == nil || len(bp.chainHash) == 0 {
		err := errors.New("DKG not finished yet")
		span.RecordError(err)
		return nil, err
	}

	_, err := inst.ProcessPartialBeacon(ctx, in)
	span.RecordError(err)
	return &drand.Empty{Metadata: bp.newMetadata()}, err
}

// PublicRand returns a public random beacon according to the request. If the Round
// field is 0, then it returns the last one generated.
func (bp *BeaconProcess) PublicRand(ctx context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	ctx, span := tracer.NewSpan(ctx, "bp.PublicRand")
	defer span.End()

	var addr = net.RemoteAddress(ctx)

	bp.state.RLock()
	defer bp.state.RUnlock()

	if bp.beacon == nil || len(bp.chainHash) == 0 {
		return nil, errors.New("drand: beacon generation not started yet")
	}
	beaconResp, err := bp.beacon.Store().Last(ctx)
	if wanted := in.GetRound(); err == nil && wanted == beaconResp.GetRound()+1 {
		// we got a request for the next round about to be produced, let's honor it with a callback
		// we cancel after period since we should never wait so long
		cctx, cancel := context.WithTimeout(ctx, bp.group.Period)
		defer cancel()
		waitlist := make(chan *common.Beacon, 1)
		rnd := make([]byte, sha256.Size)
		_, err = rand.Read(rnd)
		if err != nil {
			_ = copy(rnd, beaconResp.GetRandomness())
		}
		var mu sync.Mutex
		// the closed bool is only ever called if the same callback is re-added,
		// this has 0 chances of happening thanks to the rnd in the callback ID.
		fn := func(b *common.Beacon, _ bool) {
			mu.Lock()
			defer mu.Unlock()
			if cctx.Err() != nil {
				return
			}
			cancel()
			// we can remove our callback as soon as it's executing once
			bp.beacon.Store().RemoveCallback(addr + hex.EncodeToString(rnd))

			if b.GetRound() == wanted {
				waitlist <- b
			}
			// if we didn't get the right beacon something is off anyway, close
			close(waitlist)
		}
		bp.beacon.Store().AddCallback(addr+hex.EncodeToString(rnd), fn)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("ctx Done in PublicRand: %w", ctx.Err())
		case b, ok := <-waitlist:
			if ok {
				beaconResp, err = b, nil
			} else {
				beaconResp, err = nil, fmt.Errorf("failed to wait for beacon %d", wanted)
			}
		}
	} else if wanted > 0 {
		// fetch the correct entry or the next one if not found
		// we overwrite beaconResp and err with the correct one
		beaconResp, err = bp.beacon.Store().Get(ctx, wanted)
	}
	if err != nil || beaconResp == nil {
		bp.log.Debugw("", "public_rand", "unstored_beacon", "round", in.GetRound(), "from", addr)
		return nil, fmt.Errorf("can't retrieve beacon %d: %w", in.GetRound(), err)
	}
	bp.log.Debugw("", "public_rand", addr, "round", beaconResp.Round, "reply", beaconResp.String())

	response := beaconToProto(beaconResp)
	response.Metadata = bp.newMetadata()

	return response, nil
}

// a proxy type so public streaming request can use the same logic as in private
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
		PreviousSignature: b.PreviousSignature,
		Randomness:        crypto.RandomnessFromSignature(b.Signature),
		Metadata:          b.Metadata,
	})
}

// PublicRandStream exports a stream of new beacons as they are generated over gRPC
func (bp *BeaconProcess) PublicRandStream(req *drand.PublicRandRequest, stream drand.Public_PublicRandStreamServer) error {
	bp.state.RLock()
	if bp.beacon == nil || len(bp.chainHash) == 0 {
		bp.state.RUnlock()
		return errors.New("beacon has not started on this node yet")
	}
	bp.state.RUnlock()

	store := bp.beacon.Store()
	proxyReq := &proxyRequest{
		req,
	}
	// make sure we have the correct metadata
	proxyReq.Metadata = bp.newMetadata()
	proxyStr := &proxyStream{stream}
	return beacon.SyncChain(bp.log.Named("PublicRandStream"), store, proxyReq, proxyStr)
}

// ChainInfo replies with the chain information this node participates to
func (bp *BeaconProcess) ChainInfo(ctx context.Context, _ *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	_, span := tracer.NewSpan(ctx, "bp.ChainInfo")
	defer span.End()

	bp.state.RLock()
	group := bp.group
	chainHash := bp.chainHash
	bp.state.RUnlock()
	if group == nil || len(chainHash) == 0 {
		return nil, ErrNoGroupSetup
	}

	response := chain2.NewChainInfo(group).ToProto(bp.newMetadata())

	return response, nil
}

// SyncChain is an inter-node protocol that replies to a syncing request from a
// given round
func (bp *BeaconProcess) SyncChain(req *drand.SyncRequest, stream drand.Protocol_SyncChainServer) error {
	bp.state.RLock()
	logger := bp.log.Named("SyncChain")
	b := bp.beacon
	c := bp.chainHash
	if b == nil || len(c) == 0 {
		logger.Errorw("Received a SyncRequest, but no beacon handler is set yet", "request", req)
		bp.state.RUnlock()
		return fmt.Errorf("no beacon handler available")
	}
	store := b.Store()
	// we cannot just defer Unlock because beacon.SyncChain can run for a long time
	bp.state.RUnlock()

	return beacon.SyncChain(logger, store, req, stream)
}

// GetIdentity returns the identity of this drand node
func (bp *BeaconProcess) GetIdentity(ctx context.Context, _ *drand.IdentityRequest) (*drand.IdentityResponse, error) {
	_, span := tracer.NewSpan(ctx, "bp.GetIdentity")
	defer span.End()

	i := bp.priv.Public.ToProto()

	response := &drand.IdentityResponse{
		Address:    i.Address,
		Key:        i.Key,
		Signature:  i.Signature,
		Metadata:   bp.newMetadata(),
		SchemeName: bp.priv.Scheme().String(),
	}
	return response, nil
}

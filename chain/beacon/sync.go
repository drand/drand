package beacon

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	proto "github.com/drand/drand/protobuf/drand"
)

// SyncStore allows to follow a chain from other nodes and replies to syncing
// requests.
type Syncer interface {
	// Follow is a blocking call that continuously fetches the beacon from the
	// given nodes, until the context is cancelled.
	Follow(c context.Context) (chan *chain.Beacon, error)
	SyncChain(req *proto.SyncRequest, p proto.Protocol_SyncChainServer) error
}

// syncer implements the Syncer interface
type syncer struct {
	l         log.Logger
	store     CallbackStore
	info      *chain.Info
	nodes     []net.Peer
	client    net.ProtocolClient
	following bool
	sync.Mutex
}

// NewSyncer returns a syncer implementation
func NewSyncer(l log.Logger, s CallbackStore, info *chain.Info, client net.ProtocolClient, nodes []net.Peer) *syncer {
	return &syncer{
		store:  s,
		info:   info,
		nodes:  nodes,
		client: client,
		l:      l,
	}
}

func (s *syncer) Follow(c context.Context) error {
	s.Lock()
	if s.following {
		s.Unlock()
		return errors.New("already following chain")
	}
	s.following = true
	s.Unlock()
	defer func() {
		s.Lock()
		s.following = false
		s.Unlock()
	}()

	last, err := s.store.Last()
	if err != nil {
		return fmt.Errorf("error loading last beacon: %s", err)
	}
	fromRound := last.Round
	// shuffle through the nodes
	for _, n := range rand.Perm(len(s.nodes)) {
		node := s.nodes[n]
		cnode, cancel := context.WithCancel(c)
		beaconCh, err := s.client.SyncChain(cnode, node, &drand.SyncRequest{
			FromRound: last.Round,
		})
		if err != nil {
			s.l.Debug("sync_store", "unable_to_sync", "with_peer", node.Address(), "err", err)
			continue
		}
		s.l.Debug("sync_store", "start_follow", "with_peer", node.Address())

		for beaconPacket := range beaconCh {
			s.l.Debug("sync_store", "new_beacon_fetched", "with_peer", node.Address(), "from_round", fromRound, "got_round", beaconPacket.GetRound())
			beacon := protoToBeacon(beaconPacket)
			if err := chain.VerifyBeacon(s.info.PublicKey, beacon); err != nil {
				s.l.Debug("sync_store", "invalid_beacon", "with_peer", node.Address(), "round", beacon.Round, "err", err)
				cancel()
				break
			}
			if err := s.store.Put(beacon); err != nil {
				s.l.Debug("sync_store", "unable to save", "with_peer", node.Address(), "err", err)
				cancel()
				break
			}
			last = beacon
		}
		// see if this was a cancellation
		select {
		case <-c.Done():
			s.l.Debug("sync_store", "follow cancelled", "err?", c.Err())
			return c.Err()
		default:
		}
	}
	return errors.New("sync store tried to follow all nodes")
}

func (s *syncer) SyncChain(req *proto.SyncRequest, stream proto.Protocol_SyncChainServer) error {
	fromRound := req.GetFromRound()
	addr := net.RemoteAddress(stream.Context())
	s.l.Debug("sync_store", "sync_request", "from", addr, "from_round", fromRound)

	last, err := s.store.Last()
	if err != nil {
		return err
	}
	if last.Round < fromRound {
		return errors.New("no beacon stored above requested round")
	}

	if fromRound < last.Round {
		// first sync up from the store itself
		var err error
		s.store.Cursor(func(c chain.Cursor) {
			for bb := c.Seek(req.GetFromRound()); bb != nil; bb = c.Next() {
				if err = stream.Send(beaconToProto(bb)); err != nil {
					s.l.Debug("sync_store", "streaming_send", "err", err)
					return
				}
			}
		})
		if err != nil {
			return err
		}
	}
	var done = make(chan error, 1)
	// then register a callback to process new incoming beacons
	s.store.AddCallback(addr, func(b *chain.Beacon) {
		err := stream.Send(beaconToProto(b))
		if err != nil {
			s.l.Debug("sync_store", "streaming_send", "err", err)
			done <- nil
		}
	})
	defer s.store.RemoveCallback(addr)
	// either wait that the request cancels out or wait there's an error sending
	// to the stream
	select {
	case stream.Context().Done():
		return stream.Context().Err()
	case err := <-done:
		return err
	}
}

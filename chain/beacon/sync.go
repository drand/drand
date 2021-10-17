package beacon

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	proto "github.com/drand/drand/protobuf/drand"
)

// Syncer allows to follow a chain from other nodes and replies to syncing
// requests.
type Syncer interface {
	// Follow is a blocking call that continuously fetches the beacon from the
	// given nodes, verify the validity (chain etc) and then  stores it, until
	// the context is canceled or the round reaches upTo.  To follow
	// indefinitely, simply pass upTo = 0.
	Follow(c context.Context, upTo uint64, to []net.Peer) error
	// Syncing returns true if the syncer is currently being syncing
	Syncing() bool
	// SyncChain imeplements the server side of the syncing process
	SyncChain(req *proto.SyncRequest, p proto.Protocol_SyncChainServer) error
}

// syncer implements the Syncer interface
type syncer struct {
	l         log.Logger
	store     CallbackStore
	info      *chain.Info
	verifier  *chain.Verifier
	client    net.ProtocolClient
	following bool
	sync.Mutex
}

// NewSyncer returns a syncer implementation
func NewSyncer(l log.Logger, s CallbackStore, info *chain.Info, client net.ProtocolClient) Syncer {
	verifier := chain.NewVerifier(info.Scheme)

	return &syncer{
		store:    s,
		info:     info,
		client:   client,
		l:        l,
		verifier: verifier,
	}
}

func (s *syncer) Syncing() bool {
	s.Lock()
	defer s.Unlock()
	return s.following
}

func (s *syncer) Follow(c context.Context, upTo uint64, nodes []net.Peer) error {
	s.Lock()
	beaconID := s.info.ID

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

	s.l.Debugw("", "beacon_id", beaconID, "syncer", "starting", "up_to", upTo, "nodes", peersToString(nodes))

	// shuffle through the nodes
	for _, n := range rand.Perm(len(nodes)) {
		node := nodes[n]
		if s.tryNode(c, upTo, node) {
			return nil
		}
	}
	return errors.New("sync store tried to follow all nodes")
}

func (s *syncer) tryNode(global context.Context, upTo uint64, n net.Peer) bool {
	cnode, cancel := context.WithCancel(global)
	defer cancel()

	last, err := s.store.Last()
	if err != nil {
		return false
	}

	beaconID := s.info.ID
	beaconCh, err := s.client.SyncChain(cnode, n, &proto.SyncRequest{
		FromRound: last.Round + 1,
	})

	if err != nil {
		s.l.Debugw("", "beacon_id", beaconID, "syncer", "unable_to_sync", "with_peer", n.Address(), "err", err)
		return false
	}

	s.l.Debugw("", "beacon_id", beaconID, "syncer", "start_follow", "with_peer", n.Address(), "from_round", last.Round+1)

	for beaconPacket := range beaconCh {
		s.l.Debugw("", "beacon_id", beaconID, "syncer", "new_beacon_fetched", "with_peer", n.Address(), "from_round", last.Round+1, "got_round", beaconPacket.GetRound())
		beacon := protoToBeacon(beaconPacket)

		// verify the signature validity
		err := s.verifier.VerifyBeacon(*beacon, s.info.PublicKey)

		if err != nil {
			s.l.Debugw("", "beacon_id", beaconID, "syncer", "invalid_beacon", "with_peer", n.Address(), "round", beacon.Round, "err", err, fmt.Sprintf("%+v", beacon))
			return false
		}

		if err := s.store.Put(beacon); err != nil {
			s.l.Debugw("", "beacon_id", beaconID, "syncer", "unable to save", "with_peer", n.Address(), "err", err)
			return false
		}
		last = beacon
		if last.Round == upTo {
			s.l.Debugw("", "beacon_id", beaconID, "syncer", "syncing finished to", "round", upTo)
			return true
		}
	}
	// see if this was a cancellation from the call itself
	select {
	case <-global.Done():
		s.l.Debugw("", "beacon_id", beaconID, "syncer", "follow canceled", "err?", global.Err())
		if global.Err() == nil {
			return true
		}
		return false
	default:
	}
	return false
}

func (s *syncer) SyncChain(req *proto.SyncRequest, stream proto.Protocol_SyncChainServer) error {
	beaconID := s.info.ID
	fromRound := req.GetFromRound()
	addr := net.RemoteAddress(stream.Context())
	s.l.Debugw("", "beacon_id", beaconID, "syncer", "sync_request", "from", addr, "from_round", fromRound)

	last, err := s.store.Last()
	if err != nil {
		return err
	}
	if last.Round < fromRound {
		return fmt.Errorf("no beacon stored above requested round %d < %d", last.Round, fromRound)
	}

	if fromRound <= last.Round {
		// first sync up from the store itself
		var err error
		s.store.Cursor(func(c chain.Cursor) {
			for bb := c.Seek(fromRound); bb != nil; bb = c.Next() {
				if err = stream.Send(beaconToProto(bb)); err != nil {
					s.l.Debugw("", "beacon_id", beaconID, "syncer", "streaming_send", "err", err)
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
			s.l.Debugw("", "beacon_id", beaconID, "syncer", "streaming_send", "err", err)
			done <- nil
		}
	})
	defer s.store.RemoveCallback(addr)
	// either wait that the request cancels out or wait there's an error sending
	// to the stream
	select {
	case <-stream.Context().Done():
		return stream.Context().Err()
	case err := <-done:
		return err
	}
}

func peersToString(peers []net.Peer) string {
	var adds []string
	for _, p := range peers {
		adds = append(adds, p.Address())
	}
	return "[ " + strings.Join(adds, " - ") + " ]"
}

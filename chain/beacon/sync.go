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
	"github.com/drand/drand/protobuf/drand"
	proto "github.com/drand/drand/protobuf/drand"
)

// Syncer allows to follow a chain from other nodes and replies to syncing
// requests.
type Syncer interface {
	// Follow is a blocking call that continuously fetches the beacon from the
	// given nodes, verify the validity (chain etc) and then  stores it, until
	// the context is cancelled or the round reaches upTo.  To follow
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
	client    net.ProtocolClient
	following bool
	sync.Mutex
}

// NewSyncer returns a syncer implementation
func NewSyncer(l log.Logger, s CallbackStore, info *chain.Info, client net.ProtocolClient) Syncer {
	return &syncer{
		store:  s,
		info:   info,
		client: client,
		l:      l,
	}
}

func (s *syncer) Syncing() bool {
	s.Lock()
	defer s.Unlock()
	return s.following
}

func (s *syncer) Follow(c context.Context, upTo uint64, nodes []net.Peer) error {
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

	s.l.Debug("syncer", "starting", "up_to", upTo, "nodes", peersToString(nodes))

	last, err := s.store.Last()
	if err != nil {
		return err
	}
	fromRound := last.Round + 1
	// shuffle through the nodes
	for _, n := range rand.Perm(len(nodes)) {
		node := nodes[n]
		// we quickly pass over different nodes to catchup fast
		//cnode, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		cnode, cancel := context.WithCancel(context.Background())
		beaconCh, err := s.client.SyncChain(cnode, node, &drand.SyncRequest{
			FromRound: fromRound,
		})
		if err != nil {
			s.l.Debug("syncer", "unable_to_sync", "with_peer", node.Address(), "err", err)
			continue
		}

		s.l.Debug("syncer", "start_follow", "with_peer", node.Address(), "from_round", fromRound)

		for beaconPacket := range beaconCh {
			s.l.Debug("syncer", "new_beacon_fetched", "with_peer", node.Address(), "from_round", fromRound, "got_round", beaconPacket.GetRound())
			beacon := protoToBeacon(beaconPacket)

			// verify the signature validity
			if err := chain.VerifyBeacon(s.info.PublicKey, beacon); err != nil {
				s.l.Debug("syncer", "invalid_beacon", "with_peer", node.Address(), "round", beacon.Round, "err", err, fmt.Sprintf("%+v", beacon))
				cancel()
				break
			}

			if err := s.store.Put(beacon); err != nil {
				s.l.Debug("syncer", "unable to save", "with_peer", node.Address(), "err", err)
				cancel()
				break
			}
			last = beacon
			if last.Round == upTo {
				cancel()
				s.l.Debug("syncer", "syncing finished to", "round", upTo)
				return nil
			}
		}
		// see if this was a cancellation from the call itself
		select {
		case <-c.Done():
			s.l.Debug("syncer", "follow cancelled", "err?", c.Err())
			return c.Err()
		default:
		}
	}
	return errors.New("sync store tried to follow all nodes")
}

func (s *syncer) SyncChain(req *proto.SyncRequest, stream proto.Protocol_SyncChainServer) error {
	fromRound := req.GetFromRound()
	addr := net.RemoteAddress(stream.Context())
	s.l.Debug("syncer", "sync_request", "from", addr, "from_round", fromRound)

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
					s.l.Debug("syncer", "streaming_send", "err", err)
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
			s.l.Debug("syncer", "streaming_send", "err", err)
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

package beacon

import (
	"context"
	"errors"

	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	proto "github.com/drand/drand/protobuf/drand"
	"google.golang.org/grpc/peer"
)

// SyncChain is the server side call that reply with the beacon in order to the
// client requesting the syncing.
func (h *Handler) SyncChain(req *proto.SyncRequest, p proto.Protocol_SyncChainServer) error {
	fromRound := req.GetFromRound()
	peer, _ := peer.FromContext(p.Context())
	addr := peer.Addr.String()
	last, _ := h.chain.Last()
	h.l.Debug("received", "sync_request", "from", peer.Addr.String(), "from_round", fromRound, "head_at", last.Round)
	if last.Round < fromRound {
		return errors.New("no beacon stored above requested round")
	}
	defer h.l.Debug("sync_reply_leave", addr)
	if fromRound == 0 {
		last, err := h.chain.Last()
		if err != nil {
			return err
		}
		h.l.Debug("sync_chain_reply", addr, "from", fromRound, "reply-last", last.Round)
		return p.Send(beaconToProto(last))
	}
	var err error
	h.chain.Cursor(func(c Cursor) {
		for beacon := c.Seek(fromRound); beacon != nil; beacon = c.Next() {
			reply := beaconToProto(beacon)
			nRound, _ := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
			l, _ := h.chain.Last()
			h.l.Debug("sync_chain_reply", addr, "from", fromRound, "to", reply.Round, "head", nRound-1, "last_beacon", l.String())
			if err = p.Send(reply); err != nil {
				h.l.Debug("sync_chain_reply", "err", err)
				return
			}
			fromRound = reply.Round
		}
	})
	return err
}

// syncChain will sync from the given rounds, to the targeted round until either
// the context closes or it exhausted the list of nodes to contact to.
func syncChain(ctx context.Context, l log.Logger, safe *cryptoSafe, from *Beacon, toRound uint64, client net.ProtocolClient, out chan *Beacon) {
	fromRound := from.Round
	defer l.Debug("sync_from", fromRound, "leaving")
	request := &proto.SyncRequest{
		FromRound: fromRound + 1,
	}
	info, err := safe.GetInfo(fromRound)
	if err != nil {
		l.Error("sync_no_round_info", fromRound)
		return
	}
	ids := shuffleNodes(info.group.Nodes)
	for _, id := range ids {
		if id.Equal(info.id) {
			continue
		}
		l.Debug("sync_from", "try_sync", "to", id.Addr, "from_round", fromRound+1)
		cctx, ccancel := context.WithCancel(context.Background())
		respCh, err := client.SyncChain(cctx, id, request)
		if err != nil {
			l.Error("sync_from", fromRound+1, "error", err, "from", id.Address())
			continue
		}
		from := fromRound
		finished := func() bool {
			addr := id.Address()
			defer ccancel()
			for {
				select {
				case proto := <-respCh:
					l.Debug("sync_from", addr, "from_round", fromRound, "got_round", proto.GetRound(), "got_prev", proto.GetPreviousRound())
					// verify beacons are linked each other
					if proto.GetPreviousRound() != from {
						l.Error("sync_from", addr, "from_round", fromRound, "ignore", "future_packets", "want_round", from, "got_round", proto.GetRound())
						return false
					}
					info, err := safe.GetInfo(proto.GetRound())
					if err != nil {
						l.Error("sync_from", addr, "invalid_round_info", proto.GetRound())
						return false
					}
					err = Verify(info.pub.Commit(), proto.GetPreviousSig(), proto.GetSignature(), proto.GetPreviousRound(), proto.GetRound())
					if err != nil {
						l.Error("sync_from", addr, "invalid_beacon_sig", err, "round", proto.GetRound(), "prev", proto.GetPreviousRound())
						return false
					}
					from = proto.GetRound()
					out <- protoToBeacon(proto)
					if from == toRound {
						// we stop when we're supposed to be where we want
						return false
					}
				case <-ctx.Done():
					// we only quit when we're told or if we exhausted all peers
					return true
				}
			}
		}()
		if finished {
			return
		}
	}
}

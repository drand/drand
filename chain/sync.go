package chain

import (
	"context"
	"errors"
	"time"

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
func syncChain(ctx context.Context, l log.Logger, safe *cryptoSafe, from *Beacon, toRound uint64, client net.ProtocolClient) (chan *Beacon, error) {
	outCh := make(chan *Beacon, toRound-from.Round)
	fromRound := from.Round
	defer l.Debug("sync_from", fromRound, "leaving")

	info, err := safe.GetInfo(fromRound)
	if err != nil {
		l.Error("sync_no_round_info", fromRound)
		return nil, errors.New("no round info")
	}
	var lastBeacon = from
	ids := shuffleNodes(info.group.Nodes)
	go func() {
		defer close(outCh)
		for _, id := range ids {
			if id.Equal(info.id) {
				continue
			}
			request := &proto.SyncRequest{
				FromRound: lastBeacon.Round + 1,
			}
			l.Debug("sync_from", "try_sync", "to", id.Addr, "from_round", fromRound+1)
			cctx, ccancel := context.WithCancel(context.Background())
			respCh, err := client.SyncChain(cctx, id, request)
			if err != nil {
				l.Error("sync_from", fromRound+1, "error", err, "from", id.Address())
				ccancel()
				continue
			}
			func() {
				addr := id.Address()
				defer ccancel()
				for {
					select {
					case proto := <-respCh:
						if proto == nil {
							// because of the "select" behavior, sync returns an
							// default proto beacon - that means channel is down
							// so we log that as so
							l.Debug("sync_from", addr, "from_round", fromRound, "sync_stopped")
							return
						}

						l.Debug("sync_from", addr, "from_round", fromRound, "got_round", proto.GetRound())
						newBeacon := protoToBeacon(proto)
						if !isAppendable(lastBeacon, newBeacon) {
							l.Error("sync_from", addr, "from_round", fromRound, "want_round", lastBeacon.Round+1, "got_round", newBeacon.Round)
							return
						}
						info, err := safe.GetInfo(newBeacon.Round)
						if err != nil {
							l.Error("sync_from", addr, "invalid_round_info", newBeacon.Round)
							return
						}
						err = VerifyBeacon(info.pub.Commit(), newBeacon)
						if err != nil {
							l.Error("sync_from", addr, "invalid_beacon_sig", err, "round", newBeacon.Round)
							return
						}
						lastBeacon = newBeacon
						outCh <- newBeacon
					case <-time.After(MaxSyncWaitTime):
						return
					case <-ctx.Done():
						return
					}
				}
			}()
			if lastBeacon.Round == toRound {
				return
			}
		}
	}()
	return outCh, nil
}

package dkg

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/drand/drand/v2/internal/net"
	"github.com/drand/drand/v2/internal/util"
	drand "github.com/drand/drand/v2/protobuf/dkg"
)

// gossip marks a DKG packet as seen and sends it to the other parties in the network (that are passed in)
func (d *Process) gossip(
	me *drand.Participant,
	recipients []*drand.Participant,
	packet *drand.GossipPacket,
) chan error {
	// we first filter the recipients to avoid a malicious broadcaster sending `nil`
	recipients = util.Filter(recipients, func(p *drand.Participant) bool {
		return p != nil && p.Address != ""
	})
	// we don't want to gossip to our own node
	recipients = util.Without(recipients, me)
	// we need +1 in case it's 0
	errChan := make(chan error, len(recipients)+1)

	if len(recipients) == 0 {
		d.log.Warnw("gossip had no recipients")
		errChan <- errors.New("gossip recipients was empty")
		close(errChan)
		return errChan
	}

	if packet.Metadata == nil {
		d.log.Errorw("packet.Metadata was nil, aborting gossip")
		errChan <- errors.New("cannot process a packet without metadata")
		close(errChan)
		return errChan
	}

	// add the packet to the SeenPackets set,
	// so we don't try and reprocess it when it gets gossiped back to us!
	packetSig := hex.EncodeToString(packet.Metadata.Signature)
	d.SeenPackets[packetSig] = true

	wg := sync.WaitGroup{}
	wg.Add(len(recipients))

	for _, participant := range recipients {
		p := participant
		if p.Address == me.Address {
			d.log.Errorw("gossip recipient was containing ourselves, please report this")
			wg.Done()
			continue
		}

		// attempt to gossip with other peers
		go func() {
			err := sendToPeer(d.internalClient, p, packet)
			if err != nil {
				d.log.Warnw("tried gossiping a packet but failed", "addr", p.Address, "packet", packetSig[0:8], "err", err)
				errChan <- fmt.Errorf("error sending packet to %s: %w", p.Address, err)
			}
			wg.Done()
		}()
	}

	// signal the done channel when all the gossips + retries have been completed
	go func() {
		wg.Wait()
		d.log.Debugw("dkg gossip completed")
		close(errChan)
	}()

	return errChan
}

// (7*8/2)*200=5.6s max
var retries = 7
var backoff = 200 * time.Millisecond

func sendToPeer(client net.DKGClient, p *drand.Participant, packet *drand.GossipPacket) error {
	peer := net.CreatePeer(p.Address)
	var err error
	for i := 0; i < retries; i++ {
		// we use a separate context here, so it can continue outside the lifecycle of the gRPC request
		// that spawned the gossip request
		_, err = client.Packet(context.Background(), peer, packet)
		if err == nil {
			break
		}
		time.Sleep(time.Duration(i+1) * backoff)
	}
	return err
}

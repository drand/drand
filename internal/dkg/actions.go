package dkg

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/drand/drand/internal/net"
	"github.com/drand/drand/internal/util"
	"github.com/drand/drand/protobuf/drand"
)

// gossip marks a DKG packet as seen and sends it to the other parties in the network (that are passed in)
//
//nolint:gocritic // ewww the linter wants me to use named parameters
func (d *Process) gossip(
	me *drand.Participant,
	recipients []*drand.Participant,
	packet *drand.GossipPacket,
) (chan bool, chan error) {
	done := make(chan bool, 1)
	errChan := make(chan error, 1)

	// we first filter the recipients to avoid a malicious broadcaster sending `nil`
	recipients = util.Filter(recipients, func(p *drand.Participant) bool {
		return p != nil && p.Address != ""
	})

	if len(recipients) == 0 {
		done <- true
		return done, errChan
	}

	if packet.Metadata == nil {
		errChan <- errors.New("cannot process a packet without metadata")
		return done, errChan
	}

	// add the packet to the SeenPackets set,
	// so we don't try and reprocess it when it gets gossiped back to us!
	packetSig := hex.EncodeToString(packet.Metadata.Signature)
	d.SeenPackets[packetSig] = true

	wg := sync.WaitGroup{}

	// we don't want to gossip to our own node
	if util.Contains(recipients, me) {
		wg.Add(len(recipients) - 1)
	} else {
		wg.Add(len(recipients))
	}
	for _, participant := range recipients {
		p := participant
		if p.Address == me.Address {
			continue
		}

		// attempt to gossip with exponential backoff
		go func() {
			err := sendToPeer(d.internalClient, p, packet)
			if err != nil {
				d.log.Warnw("tried gossiping a packet but failed", "addr", p.Address, "packet", packetSig[0:8], "err", err)
				errChan <- err
			}
			wg.Done()
		}()
	}

	// signal the done channel when all the gossips + retries have been completed
	go func() {
		wg.Wait()
		done <- true
	}()

	return done, errChan
}

func sendToPeer(client net.DKGClient, p *drand.Participant, packet *drand.GossipPacket) error {
	retries := 8
	backoff := 250 * time.Millisecond

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

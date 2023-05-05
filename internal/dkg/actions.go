package dkg

import (
	"context"
	"encoding/hex"
	"sync"
	"time"

	"github.com/drand/drand/internal/net"
	"github.com/drand/drand/protobuf/drand"
)

//nolint:gocritic // ewww the linter wants me to use named parameters
func (d *Process) gossip(
	beaconID string,
	me *drand.Participant,
	recipients []*drand.Participant,
	packet *drand.GossipPacket,
	terms *drand.ProposalTerms,
) (chan bool, chan error) {
	done := make(chan bool, 1)
	errChan := make(chan error, 1)

	// first we sign the message and attach it as metadata
	metadata, err := d.signMessage(beaconID, packet, terms)
	if err != nil {
		errChan <- err
		return done, errChan
	}
	packet.Metadata = metadata

	// add the packet to the SeenPackets set,
	// so we don't try and reprocess it when it gets gossiped back to us!
	packetSig := hex.EncodeToString(metadata.Signature)
	d.SeenPackets[packetSig] = true

	wg := sync.WaitGroup{}

	// we aren't gossiping to our own node, so we can remove one
	wg.Add(len(recipients) - 1)
	for _, participant := range recipients {
		p := participant
		if p.Address == me.Address {
			continue
		}

		// attempt to gossip with exponential backoff
		go func() {
			err := sendToPeer(d.internalClient, p, packet)
			if err != nil {
				d.log.Warnw("tried gossiping a packet but failed", "addr", p.Address, "err", err)
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
	retries := 10
	backoff := 250 * time.Millisecond

	peer := net.CreatePeer(p.Address, p.Tls)
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

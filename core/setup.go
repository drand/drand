package core

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	control "github.com/drand/drand/protobuf/drand"
	proto "github.com/drand/drand/protobuf/drand"
	clock "github.com/jonboulle/clockwork"
)

// setupManager takes care of setting up a new DKG network from the perspective
// of the "leader" that collectts all info.
// General outline is like:
// From client side:
// * They need to know leader address (and later on a secret)
// * They run drand start <...>
// * They run drand share --connect <my address>
// Leader:
// * Runs drand start <...>
// * Runs drand share --leader --nodes 10 --threshold 6 --timeout 1m --start-in 10m
// 		- This commands need to be ran before the clients do it
//
// Then
// * Leader receives keys one by one, when it has 10 different ones, it creates
// the group file, with a genesis time that is current() + 10m
// * Leader sends group file to nodes and already start sending the first DKG
// packet
// * Node verify they are included and if so, run the DKG as well (processing
// the first packet of the leader will make them broadcast their deals)
// Once dkg is finished, all nodes wait for the genesis time to start the
// randomness generation
type setupManager struct {
	sync.Mutex
	expected     int
	thr          int
	startIn      time.Duration
	dkgTimeout   uint64
	clock        clock.Clock
	received     []*key.Identity
	leaderKey    *key.Identity
	verifySecret func(string) bool
	verifyKeys   func([]*key.Identity) bool
	l            log.Logger

	startDKG  chan *key.Group
	pushKeyCh chan pushKey
	doneCh    chan bool
}

func newSetupManager(l log.Logger, c clock.Clock, leaderKey *key.Identity, in *control.InitDKGPacket) (*setupManager, error) {
	n, thr, dkgTimeout, err := validInitPacket(in)
	if err != nil {
		return nil, err
	}
	// leader uses this only
	start, err := time.ParseDuration(in.GetInfo().GetStartIn())
	if err != nil {
		return nil, fmt.Errorf("invalid start-in: %v", err)
	}
	// leave at least 2mn
	if start.Seconds() < DefaultStartIn.Seconds() {
		return nil, fmt.Errorf("too small start-in: %d < %d (minimum)", start.Seconds(), DefaultStartIn.Seconds())
	}
	secret := in.GetInfo().GetSecret()
	verifySecret := func(given string) bool {
		// XXX reason for the function is that we might want to do more
		// elaborate things later like a separate secret to each individual etc
		return given == secret
	}
	verifyKeys := func(keys []*key.Identity) bool {
		// XXX Later we can add specific name list of DNS, or prexisting
		// keys..
		return true
	}

	sm := &setupManager{
		startIn:      start,
		expected:     n,
		thr:          thr,
		dkgTimeout:   uint64(dkgTimeout.Seconds()),
		l:            l,
		startDKG:     make(chan *key.Group, 1),
		pushKeyCh:    make(chan pushKey, n),
		verifySecret: verifySecret,
		verifyKeys:   verifyKeys,
		doneCh:       make(chan bool, 1),
		clock:        c,
	}
	go sm.run()
	return sm, nil
}

type pushKey struct {
	addr     string
	id       *key.Identity
	channels groupReceiver
}

// ReceivedKey takes a newly received identity and return two channels:
// receiver.WaitGroup to receive the group once ready to send back to peer
// receiver.DoneCh to notify the setup manager the group is sent. This last
// channel is to make sure the group is sent to every registered participants
// before notifying the leader to start the dkg.
func (s *setupManager) ReceivedKey(addr string, p *proto.PrepareDKGPacket) (*groupReceiver, error) {
	s.Lock()
	defer s.Unlock()
	// verify informations are correct
	if s.expected != int(p.GetExpected()) {
		return nil, fmt.Errorf("expected nodes %d vs given %d", s.expected, p.GetExpected())
	}
	if s.thr != int(p.GetThreshold()) {
		return nil, fmt.Errorf("expected threshold %s vs given %d", s.thr, p.GetThreshold())
	}
	dkgTimeout := p.GetDkgTimeout()
	if s.dkgTimeout != dkgTimeout {
		return nil, fmt.Errorf("expected dkg timeout %d vs given %d", s.dkgTimeout, dkgTimeout)
	}

	if !s.verifySecret(p.GetSecretProof()) {
		return nil, errors.New("shared secret is incorrect")
	}

	newID, err := protoToIdentity(p.GetNode())
	if err != nil {
		s.l.Info("setup", "error_decoding", "id", addr, err)
		return nil, fmt.Errorf("invalid id: %v", err)
	}

	receiver := groupReceiver{
		WaitGroup: make(chan *key.Group, 1),
		DoneCh:    make(chan bool, 1),
	}
	s.pushKeyCh <- pushKey{
		addr:     addr,
		id:       newID,
		channels: receiver,
	}
	return &receiver, nil
}

type groupReceiver struct {
	// channel over which to send the group when ready
	WaitGroup chan *key.Group
	// channel over which leader notifies it has sent group to the member
	DoneCh chan bool
}

func (s *setupManager) run() {
	var inKeys = make([]*key.Identity, 0, s.expected)
	inKeys = append(inKeys, s.leaderKey)
	// - 1 because leader doesn't wait on the same channel
	var receivers = make([]groupReceiver, 0, s.expected-1)
	for {
		select {
		case pk := <-s.pushKeyCh:
			// verify it's not in the list we have
			var found bool
			for _, id := range inKeys {
				sameAddr := id.Address() == pk.id.Address()
				// lazy eval
				sameKey := func() bool { return id.Key.Equal(pk.id.Key) }
				if sameAddr || sameKey() {
					found = true
					s.l.Debug("setup", "duplicate", "ip", pk.addr, "addr", pk.id.String())
					// notify the waiter that it's not working
					close(pk.channels.WaitGroup)
					break
				}
			}
			// we already received this key
			if found {
				break
			}
			s.l.Debug("setup", "added", "key", pk.id.String())
			inKeys = append(inKeys, pk.id)
			receivers = append(receivers, pk.channels)

			// create group if we have enough keys
			if len(s.received) == s.expected {
				if s.verifyKeys(s.received) {
					// we dont want to receive others
					s.doneCh <- true
					// go send the keys back to all participants
					s.createAndSend(inKeys, receivers)
					// job is done
					return
				}
			}
		case <-s.doneCh:
			s.l.Debug("setup", "done")
			return
		}
	}

}

func (s *setupManager) createAndSend(keys []*key.Identity, receivers []groupReceiver) {
	// create group
	// genesis time is specified w.r.t. to the start in time
	genesis := s.clock.Now().Add(s.startIn).Unix()
	group := key.NewGroup(keys, s.thr, genesis)
	s.l.Debug("setup", "created_group")
	fmt.Printf("Generated group:\n%s\n", group.String())
	// send to all connections that wait for the group
	for _, receiver := range receivers {
		receiver.WaitGroup <- group
	}
	// wait that leader has sent to all connections
	for _, receiver := range receivers {
		<-receiver.DoneCh
	}
	// signal the leader it's ready to run the DKG
	s.startDKG <- group
	// job is done
}

func (s *setupManager) WaitForGroupSent() chan *key.Group {
	return s.startDKG
}

// StopPreemptively is to be called if something is wrong *before* the
// group is created. In normal cases, setupManager will stop itself.
func (s *setupManager) StopPreemptively() {
	s.doneCh <- true
}

func validInitPacket(in *control.InitDKGPacket) (n int, thr int, dkg time.Duration, err error) {
	n = int(in.GetInfo().GetNodes())
	thr = int(in.GetInfo().GetThreshold())
	if thr < key.MinimumT(n) {
		err = fmt.Errorf("invalid thr: need %d got %d", thr, key.MinimumT(n))
		return
	}
	dkg, err = time.ParseDuration(in.GetInfo().GetTimeout())
	if err != nil {
		err = fmt.Errorf("invalid dkg timeout: %v", err)
		return
	}
	return
}

package core

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
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
	beaconOffset time.Duration
	beaconPeriod time.Duration
	dkgTimeout   uint64
	clock        clock.Clock
	leaderKey    *key.Identity
	verifySecret func(string) bool
	verifyKeys   func([]*key.Identity) bool
	l            log.Logger

	isResharing bool
	oldGroup    *key.Group
	oldHash     []byte

	startDKG  chan *key.Group
	pushKeyCh chan pushKey
	doneCh    chan bool
}

func newDKGSetup(l log.Logger, c clock.Clock, leaderKey *key.Identity, beaconPeriod uint32, in *control.SetupInfoPacket) (*setupManager, error) {
	n, thr, dkgTimeout, err := validInitPacket(in)
	if err != nil {
		return nil, err
	}
	secret := in.GetSecret()
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
	offset := time.Duration(in.GetBeaconOffset()) * time.Second
	if in.GetBeaconOffset() == 0 {
		offset = DefaultGenesisOffset
	}

	sm := &setupManager{
		expected:     n,
		thr:          thr,
		beaconOffset: offset,
		beaconPeriod: time.Duration(beaconPeriod) * time.Second,
		dkgTimeout:   uint64(dkgTimeout.Seconds()),
		l:            l,
		startDKG:     make(chan *key.Group, 1),
		pushKeyCh:    make(chan pushKey, n),
		verifySecret: verifySecret,
		verifyKeys:   verifyKeys,
		doneCh:       make(chan bool, 1),
		clock:        c,
		leaderKey:    leaderKey,
	}
	return sm, nil
}

func newReshareSetup(l log.Logger, c clock.Clock, leaderKey *key.Identity, oldGroup *key.Group, in *control.InitResharePacket) (*setupManager, error) {
	// period isn't included for resharing since we keep the same period
	beaconPeriod := uint32(oldGroup.Period.Seconds())
	sm, err := newDKGSetup(l, c, leaderKey, beaconPeriod, in.GetInfo())
	if err != nil {
		return nil, err
	}

	sm.oldGroup = oldGroup
	sm.oldHash = oldGroup.Hash()
	sm.isResharing = true
	offset := time.Duration(in.GetInfo().GetBeaconOffset()) * time.Second
	if offset == 0 {
		offset = DefaultResharingOffset
	}
	sm.beaconOffset = offset
	return sm, nil
}

type pushKey struct {
	addr string
	id   *key.Identity
}

// ReceivedKey takes a newly received identity and return two channels:
// receiver.WaitGroup to receive the group once ready to send back to peer
// receiver.DoneCh to notify the setup manager the group is sent. This last
// channel is to make sure the group is sent to every registered participants
// before notifying the leader to start the dkg.
func (s *setupManager) ReceivedKey(addr string, p *proto.SignalDKGPacket) error {
	s.Lock()
	defer s.Unlock()
	if !s.verifySecret(p.GetSecretProof()) {
		return errors.New("shared secret is incorrect")
	}
	if s.isResharing {
		if !bytes.Equal(s.oldHash, p.GetPreviousGroupHash()) {
			return errors.New("inconsistent previous group hash")
		}
	}

	newID, err := key.IdentityFromProto(p.GetNode())
	if err != nil {
		s.l.Info("setup", "error_decoding", "id", addr, "err", err)
		return fmt.Errorf("invalid id: %v", err)
	}

	s.l.Debug("setup", "received_new_key", "id", newID.String())

	s.pushKeyCh <- pushKey{
		addr: addr,
		id:   newID,
	}
	return nil
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
					break
				}
			}
			// we already received this key
			if found {
				break
			}
			inKeys = append(inKeys, pk.id)
			s.l.Debug("setup", "added", "key", pk.id.String(), "have", fmt.Sprintf("%d/%d", len(inKeys), s.expected))

			// create group if we have enough keys
			if len(inKeys) == s.expected {
				if s.verifyKeys(inKeys) {
					// we dont want to receive others
					s.doneCh <- true
					// we dont want to receive others
					// go send the keys back to all participants
					s.createAndSend(inKeys)
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

func (s *setupManager) createAndSend(keys []*key.Identity) {
	// create group
	var group *key.Group
	if !s.isResharing {
		genesis := s.clock.Now().Add(s.beaconOffset).Unix()
		// round the genesis time to a period modulo
		ps := int64(s.beaconPeriod.Seconds())
		genesis = genesis + (ps - genesis%ps)
		group = key.NewGroup(keys, s.thr, genesis, s.beaconPeriod)
	} else {
		genesis := s.oldGroup.GenesisTime
		atLeast := s.clock.Now().Add(s.beaconOffset).Unix()
		// transitioning to the next round time that is at least
		// "DefaultResharingOffset" time from now.
		_, transition := chain.NextRound(atLeast, s.beaconPeriod, s.oldGroup.GenesisTime)
		group = key.NewGroup(keys, s.thr, genesis, s.beaconPeriod)
		group.TransitionTime = transition
		group.GenesisSeed = s.oldGroup.GetGenesisSeed()
	}
	s.l.Debug("setup", "created_group")
	fmt.Printf("Generated group:\n%s\n", group.String())
	// signal the leader it's ready to run the DKG
	s.startDKG <- group
}

func (s *setupManager) WaitGroup() chan *key.Group {
	return s.startDKG
}

// StopPreemptively is to be called if something is wrong *before* the group is
// created. In normal cases, setupManager will stop itself.
func (s *setupManager) StopPreemptively() {
	s.doneCh <- true
}

func validInitPacket(in *control.SetupInfoPacket) (n int, thr int, dkg time.Duration, err error) {
	n = int(in.GetNodes())
	thr = int(in.GetThreshold())
	if thr < key.MinimumT(n) {
		err = fmt.Errorf("invalid thr: %d nodes, need thr %d got %d", n, thr, key.MinimumT(n))
		return
	}
	dkg = time.Duration(in.GetTimeout()) * time.Second
	return
}

// setupReceiver is a simple struct that expects to receive a group information
// to setup a new DKG. When it receives it from the coordinator, it pass it
// along the to the logic waiting to start the DKG.
type setupReceiver struct {
	ch     chan *drand.DKGInfoPacket
	l      log.Logger
	secret string
}

func newSetupReceiver(l log.Logger, in *control.SetupInfoPacket) *setupReceiver {
	return &setupReceiver{
		ch:     make(chan *drand.DKGInfoPacket, 1),
		l:      l,
		secret: in.GetSecret(),
	}
}

func (r *setupReceiver) PushDKGInfo(pg *drand.DKGInfoPacket) error {
	if pg.GetSecretProof() != r.secret {
		r.l.Debug("received", "invalid_secret_proof")
		return errors.New("invalid secret")
	}
	r.ch <- pg
	return nil
}

func (r *setupReceiver) WaitDKGInfo() chan *drand.DKGInfoPacket {
	return r.ch
}

func (r *setupReceiver) stop() {
	close(r.ch)
}

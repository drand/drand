package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"sync"
	"time"

	clock "github.com/jonboulle/clockwork"

	"github.com/drand/drand/chain"
	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
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
//   - This commands need to be ran before the clients do it
//
// Then
// * Leader receives keys one by one, when it has 10 different ones, it creates
// the group file, with a genesis time that is current() + 10m
// * Leader sends group file to nodes and already start sending the first DKG
// packet
// * MockNode verify they are included and if so, run the DKG as well (processing
// the first packet of the leader will make them broadcast their deals)
// Once dkg is finished, all nodes wait for the genesis time to start the
// randomness generation
type setupManager struct {
	sync.Mutex
	expected      int
	thr           int
	beaconOffset  time.Duration
	catchupPeriod time.Duration
	beaconPeriod  time.Duration
	beaconID      string
	scheme        *crypto.Scheme
	dkgTimeout    time.Duration
	clock         clock.Clock
	leaderKey     *key.Identity
	verifyKeys    func([]*key.Identity) bool
	l             log.Logger

	isResharing bool
	oldGroup    *key.Group
	oldHash     []byte

	startDKG     chan *key.Group
	pushKeyCh    chan pushKey
	doneCh       chan bool
	hashedSecret []byte
}

type setupConfig struct {
	l             log.Logger
	c             clock.Clock
	leaderKey     *key.Identity
	beaconPeriod  uint32
	catchupPeriod uint32
	beaconID      string
	scheme        *crypto.Scheme
	info          *drand.SetupInfoPacket
}

func newDKGSetup(c *setupConfig) (*setupManager, error) {
	n, thr, dkgTimeout, err := validInitPacket(c.info)
	if err != nil {
		return nil, err
	}
	secret := hashSecret(c.info.GetSecret())
	verifyKeys := func(keys []*key.Identity) bool {
		// XXX Later we can add specific name list of DNS, or preexisting
		// keys..
		return true
	}
	offset := time.Duration(c.info.GetBeaconOffset()) * time.Second
	if c.info.GetBeaconOffset() == 0 {
		offset = DefaultGenesisOffset
	}

	sm := &setupManager{
		expected:      n,
		thr:           thr,
		beaconOffset:  offset,
		beaconPeriod:  time.Duration(c.beaconPeriod) * time.Second,
		catchupPeriod: time.Duration(c.catchupPeriod) * time.Second,
		scheme:        c.scheme,
		beaconID:      c.beaconID,
		dkgTimeout:    dkgTimeout,
		l:             c.l,
		startDKG:      make(chan *key.Group, 1),
		pushKeyCh:     make(chan pushKey, n),
		verifyKeys:    verifyKeys,
		doneCh:        make(chan bool, 1),
		clock:         c.c,
		leaderKey:     c.leaderKey,
		hashedSecret:  secret,
	}
	return sm, nil
}

func newReshareSetup(
	l log.Logger,
	c clock.Clock,
	leaderKey *key.Identity,
	oldGroup *key.Group,
	in *drand.InitResharePacket,
) (*setupManager, error) {
	// period isn't included for resharing since we keep the same period
	beaconPeriod := uint32(oldGroup.Period.Seconds())
	// we know it was properly set and verified earlier
	beaconID := oldGroup.ID

	catchupPeriod := in.CatchupPeriod
	if !in.CatchupPeriodChanged {
		catchupPeriod = uint32(oldGroup.CatchupPeriod.Seconds())
	}

	sm, err := newDKGSetup(&setupConfig{
		l.Named("ResharingDKGSetup"),
		c, leaderKey, beaconPeriod,
		catchupPeriod, beaconID,
		oldGroup.Scheme, in.GetInfo(),
	})
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

// ReceivedKey takes a newly received identity from a peer joining a DKG
func (s *setupManager) ReceivedKey(addr string, p *drand.SignalDKGPacket) error {
	s.Lock()
	defer s.Unlock()
	if !correctSecret(s.hashedSecret, p.GetSecretProof()) {
		s.l.Errorw("SignalDKGPacket received with invalid shared secret", "from_addr", addr, "packet_addr", p.GetNode().GetAddress())
		return errors.New("shared secret is incorrect")
	}

	if s.isResharing {
		if pOldHash := p.GetPreviousGroupHash(); !bytes.Equal(s.oldHash, pOldHash) {
			s.l.Errorw("inconsistent previous group hash", "oldHash", s.oldHash, "packet_oldHash", pOldHash)
			return errors.New("inconsistent previous group hash")
		}
	}

	newID, err := key.IdentityFromProto(p.GetNode(), s.scheme)
	if err != nil {
		s.l.Errorw("error decoding in ReceivedKey", "id", addr, "err", err)
		return fmt.Errorf("invalid id: %w", err)
	}

	if err := newID.ValidSignature(); err != nil {
		s.l.Errorw("invalid identity signature in ReceivedKey", "id", addr, "err", err)
		return errors.Join(fmt.Errorf("invalid sig: %w", err), key.ErrInvalidKeyScheme)
	}

	s.l.Debugw("", "setup", "received_new_key", "id", newID.String())

	s.pushKeyCh <- pushKey{
		addr: addr,
		id:   newID,
	}
	return nil
}

func (s *setupManager) run() {
	logger := s.l.Named("setupManagerRun")
	defer close(s.startDKG)
	inKeys := make([]*key.Identity, 0, s.expected)
	inKeys = append(inKeys, s.leaderKey)
	for {
		select {
		case pk := <-s.pushKeyCh:
			// verify it's not in the list we have
			var found bool
			for _, id := range inKeys {
				if id.Address() == pk.id.Address() {
					found = true
					logger.Debugw("", "setup", "duplicate", "ip", pk.addr, "addr", pk.id.String())
					break
				} else if id.Key.Equal(pk.id.Key) {
					found = true
					logger.Debugw("", "setup", "duplicate", "ip", pk.addr, "addr", pk.id.String())
					break
				}
			}
			// we already received this key
			if found {
				break
			}
			inKeys = append(inKeys, pk.id)
			logger.Debugw("", "setup", "added", "key", pk.id.String(), "have", fmt.Sprintf("%d/%d", len(inKeys), s.expected))

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
			logger.Debugw("", "setup", "preempted", "collected_keys", len(inKeys))
			return
		}
	}
}

func (s *setupManager) createAndSend(keys []*key.Identity) {
	// create group
	var group *key.Group
	totalDKG := s.dkgTimeout*3 + s.beaconOffset
	if !s.isResharing {
		genesis := s.clock.Now().Add(totalDKG).Unix()
		// round the genesis time to a period modulo
		ps := int64(s.beaconPeriod.Seconds())
		genesis += (ps - genesis%ps)
		group = key.NewGroup(keys, s.thr, genesis, s.beaconPeriod, s.catchupPeriod, s.scheme, s.beaconID)
	} else {
		genesis := s.oldGroup.GenesisTime

		// Transition will happen 2 rounds after the minimum time the DKG takes.
		// This will allow nodes to not produce "bls: invalid signature" errors from nodes in the network.
		atLeast := s.clock.Now().Add(totalDKG).Add(s.beaconPeriod).Unix()

		// transitioning to the next round time that is at least
		// "DefaultResharingOffset" time from now.
		_, transition := chain.NextRound(atLeast, s.beaconPeriod, s.oldGroup.GenesisTime)

		group = key.NewGroup(keys, s.thr, genesis, s.beaconPeriod, s.catchupPeriod, s.scheme, s.beaconID)
		group.TransitionTime = transition
		group.GenesisSeed = s.oldGroup.GetGenesisSeed()
	}
	s.l.Debugw("", "setup", "created_group")
	s.l.Debugw("Generated group", "group", group.String())
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

func validInitPacket(in *drand.SetupInfoPacket) (n, thr int, dkg time.Duration, err error) {
	n = int(in.GetNodes())
	thr = int(in.GetThreshold())
	if thr < key.MinimumT(n) {
		err = fmt.Errorf("invalid thr: %d nodes, need thr %d got %d", n, thr, key.MinimumT(n))
		return
	}
	if thr > n {
		err = fmt.Errorf("invalid thr: %d nodes, can't have thr %d", n, thr)
		return
	}
	dkg = time.Duration(in.GetTimeout()) * time.Second
	return
}

// setupReceiver is a simple struct that expects to receive a group information
// to setup a new DKG. When it receives it from the coordinator, it pass it
// along the to the logic waiting to start the DKG.
type setupReceiver struct {
	client   net.ProtocolClient
	clock    clock.Clock
	ch       chan *dkgGroup
	l        log.Logger
	leader   net.Peer
	leaderID *key.Identity
	secret   []byte
	done     bool
	version  commonutils.Version
	beaconID string
	scheme   *crypto.Scheme
}

func newSetupReceiver(version commonutils.Version, l log.Logger, c clock.Clock,
	client net.ProtocolClient, in *drand.SetupInfoPacket, sch *crypto.Scheme,
) (*setupReceiver, error) {
	beaconID := commonutils.GetCanonicalBeaconID(in.GetMetadata().GetBeaconID())

	setup := &setupReceiver{
		ch:       make(chan *dkgGroup, 1),
		l:        l,
		leader:   net.CreatePeer(in.GetLeaderAddress(), in.GetLeaderTls()),
		client:   client,
		clock:    c,
		secret:   hashSecret(in.GetSecret()),
		version:  version,
		beaconID: beaconID,
		scheme:   sch,
	}

	if err := setup.fetchLeaderKey(); err != nil {
		return nil, err
	}
	return setup, nil
}

func (r *setupReceiver) fetchLeaderKey() error {
	request := &drand.IdentityRequest{Metadata: &common.Metadata{NodeVersion: r.version.ToProto(), BeaconID: r.beaconID}}

	protoID, err := r.client.GetIdentity(context.Background(), r.leader, request)
	if err != nil {
		return err
	}

	identity := &drand.Identity{
		Signature: protoID.Signature,
		Tls:       protoID.Tls,
		Address:   protoID.Address,
		Key:       protoID.Key,
	}
	id, err := key.IdentityFromProto(identity, r.scheme)
	if err != nil {
		return err
	}

	r.leaderID = id
	return nil
}

type dkgGroup struct {
	group   *key.Group
	timeout uint32
}

// PushDKGInfo method is being called when a node received a group from the
// leader. It runs some routines verification of the group before passing it on
// to the routine that waits for the group to start the DKG.
func (r *setupReceiver) PushDKGInfo(pg *drand.DKGInfoPacket) error {
	if !correctSecret(r.secret, pg.GetSecretProof()) {
		r.l.Debugw("", "received", "invalid_secret_proof")
		return errors.New("invalid secret")
	}
	// verify things are all in order
	group, err := key.GroupFromProto(pg.NewGroup, r.scheme)
	if err != nil {
		return fmt.Errorf("group from leader invalid: %w", err)
	}
	if err := r.leaderID.Scheme.DKGAuthScheme.Verify(r.leaderID.Key, group.Hash(), pg.Signature); err != nil {
		r.l.Errorw("", "received", "group", "invalid_sig", err)
		return fmt.Errorf("invalid group sig: %w", err)
	}
	checkGroup(r.l, group)
	r.ch <- &dkgGroup{
		group:   group,
		timeout: pg.GetDkgTimeout(),
	}
	return nil
}

func (r *setupReceiver) WaitDKGInfo(ctx context.Context) (*key.Group, uint32, error) {
	select {
	case dkgGroup := <-r.ch:
		if dkgGroup == nil {
			return nil, 0, errors.New("unable to fetch group")
		}
		r.l.Debugw("", "init_dkg", "received_group")
		return dkgGroup.group, dkgGroup.timeout, nil
	case <-r.clock.After(MaxWaitPrepareDKG):
		r.l.Errorw("", "init_dkg", "wait_group", "err", "timeout")
		return nil, 0, errors.New("wait_group timeouts from coordinator")
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
}

// stop must be called in a thread safe manner
func (r *setupReceiver) stop() {
	if r.done {
		return
	}
	close(r.ch)
	r.done = true
}

// correctSecret returns true if `hashed" and the hash of `received are equal.
// It performs the comparison in constant time to avoid leaking timing
// information about the secret.
func correctSecret(hashed, received []byte) bool {
	got := hashSecret(received)
	return subtle.ConstantTimeCompare(hashed, got) == 1
}

func hashSecret(s []byte) []byte {
	hashed := sha256.Sum256(s)
	return hashed[:]
}

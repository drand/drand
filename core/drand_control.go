package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/beacon"
	"github.com/drand/drand/entropy"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share/dkg"
	vss "github.com/drand/kyber/share/vss/pedersen"
	clock "github.com/jonboulle/clockwork"
)

// errPreempted is returned on reshares when a subsequent reshare is started concurrently
var errPreempted = errors.New("time out: pre-empted")

// InitDKG take a InitDKGPacket, extracts the informations needed and wait for
// the DKG protocol to finish. If the request specifies this node is a leader,
// it starts the DKG protocol.
func (d *Drand) InitDKG(c context.Context, in *drand.InitDKGPacket) (*drand.GroupPacket, error) {
	isLeader := in.GetInfo().GetLeader()
	d.state.Lock()
	if d.dkgDone {
		d.state.Unlock()
		return nil, errors.New("dkg phase already done - call reshare")
	}
	d.state.Unlock()
	if !isLeader {
		// different logic for leader than the rest
		out, err := d.setupAutomaticDKG(c, in)
		return out, err
	}
	d.log.Info("init_dkg", "begin", "time", d.opts.clock.Now().Unix(), "leader", true)

	// setup the manager
	newSetup := func(d *Drand) (*setupManager, error) {
		return newDKGSetup(d.log, d.opts.clock, d.priv.Public, in.GetBeaconPeriod(), in.GetCatchupPeriod(), in.GetInfo())
	}

	// expect the group
	group, err := d.leaderRunSetup(newSetup)
	if err != nil {
		d.log.Error("init_dkg", "leader setup", "err", err)
		return nil, fmt.Errorf("drand: invalid setup configuration: %s", err)
	}

	// send it to everyone in the group nodes
	nodes := group.Nodes
	if err := d.pushDKGInfo([]*key.Node{}, nodes, 0, group, in.GetInfo().GetSecret(), in.GetInfo().GetTimeout()); err != nil {
		return nil, err
	}
	finalGroup, err := d.runDKG(true, group, in.GetInfo().GetTimeout(), in.GetEntropy())
	if err != nil {
		return nil, err
	}
	return finalGroup.ToProto(), nil
}

func (d *Drand) leaderRunSetup(newSetup func(d *Drand) (*setupManager, error)) (group *key.Group, err error) {
	// setup the manager
	d.state.Lock()
	if d.manager != nil {
		d.log.Info("reshare", "already_in_progress", "restart", "reshare", "old")
		d.manager.StopPreemptively()
	}
	manager, err := newSetup(d)
	d.log.Info("reshare", "newmanager")
	if err != nil {
		d.state.Unlock()
		return nil, fmt.Errorf("drand: invalid setup configuration: %s", err)
	}
	go manager.run()
	d.manager = manager
	d.state.Unlock()
	defer func() {
		// don't clear manager if pre-empted
		if err == errPreempted {
			return
		}
		d.state.Lock()
		// set back manager to nil afterwards to be able to run a new setup
		d.manager = nil
		d.state.Unlock()
	}()

	// wait to receive the keys & send them to the other nodes
	var ok bool
	select {
	case group, ok = <-manager.WaitGroup():
		if ok {
			var addr []string
			for _, k := range group.Nodes {
				addr = append(addr, k.Address())
			}
			d.log.Debug("init_dkg", "setup_phase", "keys_received", "["+strings.Join(addr, "-")+"]")
		} else {
			d.log.Debug("init_dkg", "pre-empted")
			return nil, errPreempted
		}
	case <-time.After(MaxWaitPrepareDKG):
		d.log.Debug("init_dkg", "time_out")
		manager.StopPreemptively()
		return nil, errors.New("time outs: no key received")
	}
	return group, nil
}

// runDKG setups the proper structures and protocol to run the DKG and waits
// until it finishes. If leader is true, this node sends the first packet.
func (d *Drand) runDKG(leader bool, group *key.Group, timeout uint32, randomness *drand.EntropyInfo) (*key.Group, error) {
	reader, user := extractEntropy(randomness)
	config := &dkg.Config{
		Suite:          key.KeyGroup.(dkg.Suite),
		NewNodes:       group.DKGNodes(),
		Longterm:       d.priv.Key,
		Reader:         reader,
		UserReaderOnly: user,
		FastSync:       true,
		Threshold:      group.Threshold,
		Nonce:          getNonce(group),
		Auth:           key.DKGAuthScheme,
	}
	phaser := d.getPhaser(timeout)
	board := newBroadcast(d.log, d.privGateway.ProtocolClient, d.priv.Public.Address(), group.Nodes, func(p dkg.Packet) error {
		return dkg.VerifyPacketSignature(config, p)
	})
	dkgProto, err := dkg.NewProtocol(config, board, phaser, true)
	if err != nil {
		return nil, err
	}

	d.state.Lock()
	dkgInfo := &dkgInfo{
		target: group,
		board:  board,
		phaser: phaser,
		conf:   config,
		proto:  dkgProto,
	}
	d.dkgInfo = dkgInfo
	if leader {
		d.dkgInfo.started = true
	}
	d.state.Unlock()

	if leader {
		// phaser will kick off the first phase for every other nodes so
		// nodes will send their deals
		d.log.Info("init_dkg", "START_DKG")
		go phaser.Start()
	}
	d.log.Info("init_dkg", "wait_dkg_end")
	finalGroup, err := d.WaitDKG()
	if err != nil {
		d.log.Error("init_dkg", err)
		d.state.Lock()
		if d.dkgInfo == dkgInfo {
			d.cleanupDKG()
		}
		d.state.Unlock()
		return nil, fmt.Errorf("drand: %v", err)
	}
	d.state.Lock()
	d.cleanupDKG()
	d.dkgDone = true
	d.state.Unlock()
	d.log.Info("init_dkg", "dkg_done", "starting_beacon_time", finalGroup.GenesisTime, "now", d.opts.clock.Now().Unix())
	// beacon will start at the genesis time specified
	go d.StartBeacon(false)
	return finalGroup, nil
}

func (d *Drand) cleanupDKG() {
	if d.dkgInfo != nil {
		d.dkgInfo.board.stop()
	}
	d.dkgInfo = nil
}

// runResharing setups all necessary structures to run the resharing protocol
// and waits until it finishes (or timeouts). If leader is true, it sends the
// first packet so other nodes will start as soon as they receive it.
func (d *Drand) runResharing(leader bool, oldGroup, newGroup *key.Group, timeout uint32) (*key.Group, error) {
	oldNode := oldGroup.Find(d.priv.Public)
	oldPresent := oldNode != nil
	if leader && !oldPresent {
		d.log.Error("run_reshare", "invalid", "leader", leader, "old_present", oldPresent)
		return nil, errors.New("can not be a leader if not present in the old group")
	}
	newNode := newGroup.Find(d.priv.Public)
	newPresent := newNode != nil
	config := &dkg.Config{
		Suite:        key.KeyGroup.(dkg.Suite),
		NewNodes:     newGroup.DKGNodes(),
		OldNodes:     oldGroup.DKGNodes(),
		Longterm:     d.priv.Key,
		Threshold:    newGroup.Threshold,
		OldThreshold: oldGroup.Threshold,
		FastSync:     true,
		Nonce:        getNonce(newGroup),
		Auth:         key.DKGAuthScheme,
	}
	err := func() error {
		d.state.Lock()
		defer d.state.Unlock()
		// gives the share to the dkg if we are a current node
		if oldPresent {
			if d.dkgInfo != nil {
				return errors.New("control: can't reshare from old node when DKG not finished first")
			}
			if d.share == nil {
				return errors.New("control: can't reshare without a share")
			}
			dkgShare := dkg.DistKeyShare(*d.share)
			config.Share = &dkgShare
		} else {
			// we are a new node, we want to make sure we reshare from the old
			// group public key
			config.PublicCoeffs = oldGroup.PublicKey.Coefficients
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}

	allNodes := nodeUnion(oldGroup.Nodes, newGroup.Nodes)
	board := newBroadcast(d.log, d.privGateway.ProtocolClient, d.priv.Public.Address(), allNodes, func(p dkg.Packet) error {
		return dkg.VerifyPacketSignature(config, p)
	})
	phaser := d.getPhaser(timeout)

	dkgProto, err := dkg.NewProtocol(config, board, phaser, true)
	if err != nil {
		return nil, err
	}
	info := &dkgInfo{
		target: newGroup,
		board:  board,
		phaser: phaser,
		conf:   config,
		proto:  dkgProto,
	}
	d.state.Lock()
	d.dkgInfo = info
	if leader {
		d.log.Info("dkg_reshare", "leader_start", "target_group", hex.EncodeToString(newGroup.Hash()), "index", newNode.Index)
		d.dkgInfo.started = true
	}
	d.state.Unlock()

	if leader {
		// start the protocol so everyone else follows
		// it sends to all previous and new nodes. old nodes will start their
		// phaser so they will send the deals as soon as they receive this.
		go phaser.Start()
	}

	d.log.Info("dkg_reshare", "wait_dkg_end")
	finalGroup, err := d.WaitDKG()
	if err != nil {
		d.state.Lock()
		if d.dkgInfo == info {
			d.cleanupDKG()
		}
		d.state.Unlock()
		return nil, fmt.Errorf("drand: err during DKG: %v", err)
	}
	d.log.Info("dkg_reshare", "finished", "leader", leader)
	// runs the transition of the beacon
	go d.transition(oldGroup, oldPresent, newPresent)
	return finalGroup, nil
}

// This method sends the public key to the denoted leader address and then waits
// to receive the group file. After receiving it, it starts the DKG process in
// "waiting" mode, waiting for the leader to send the first packet.
func (d *Drand) setupAutomaticDKG(_ context.Context, in *drand.InitDKGPacket) (*drand.GroupPacket, error) {
	d.log.Info("init_dkg", "begin", "leader", false)
	// determine the leader's address
	laddr := in.GetInfo().GetLeaderAddress()
	lpeer := net.CreatePeer(laddr, in.GetInfo().GetLeaderTls())
	d.state.Lock()
	if d.receiver != nil {
		d.log.Info("dkg_setup", "already_in_progress", "restart", "dkg")
		d.receiver.stop()
	}
	receiver, err := newSetupReceiver(d.log, d.opts.clock, d.privGateway.ProtocolClient, in.GetInfo())
	if err != nil {
		d.log.Error("setup", "fail", "err", err)
		d.state.Unlock()
		return nil, err
	}
	d.receiver = receiver
	d.state.Unlock()

	defer func(r *setupReceiver) {
		d.state.Lock()
		r.stop()
		if r == d.receiver {
			// if there has been no new receiver since, we set the field to nil
			d.receiver = nil
		}
		d.state.Unlock()
	}(receiver)
	// send public key to leader
	id := d.priv.Public.ToProto()
	prep := &drand.SignalDKGPacket{
		Node:        id,
		SecretProof: in.GetInfo().GetSecret(),
	}

	d.log.Debug("init_dkg", "send_key", "leader", lpeer.Address())
	nc, cancel := context.WithTimeout(context.Background(), MaxWaitPrepareDKG)
	defer cancel()

	err = d.privGateway.ProtocolClient.SignalDKGParticipant(nc, lpeer, prep)
	if err != nil {
		return nil, fmt.Errorf("drand: err when signaling key to leader: %s", err)
	}

	d.log.Debug("init_dkg", "wait_group")

	group, dkgTimeout, err := d.receiver.WaitDKGInfo(nc)
	if err != nil {
		return nil, err
	}
	if group == nil {
		d.log.Debug("init_dkg", "wait_group", "canceled", "nil_group")
		return nil, errors.New("canceled operation")
	}

	now := d.opts.clock.Now().Unix()
	if group.GenesisTime < now {
		d.log.Error("genesis", "invalid", "given", group.GenesisTime)
		return nil, errors.New("control: group with genesis time in the past")
	}

	node := group.Find(d.priv.Public)
	if node == nil {
		d.log.Error("init_dkg", "absent_public_key_in_received_group")
		return nil, errors.New("drand: public key not found in group")
	}
	d.state.Lock()
	d.index = int(node.Index)
	d.state.Unlock()

	// run the dkg
	finalGroup, err := d.runDKG(false, group, dkgTimeout, in.GetEntropy())
	if err != nil {
		return nil, err
	}
	return finalGroup.ToProto(), nil
}

// similar to setupAutomaticDKG but with additional verification and information
// w.r.t. to the previous group
func (d *Drand) setupAutomaticResharing(_ context.Context, oldGroup *key.Group, in *drand.InitResharePacket) (*drand.GroupPacket, error) {
	oldHash := oldGroup.Hash()
	// determine the leader's address
	laddr := in.GetInfo().GetLeaderAddress()
	lpeer := net.CreatePeer(laddr, in.GetInfo().GetLeaderTls())
	d.state.Lock()
	if d.receiver != nil {
		if !in.GetInfo().GetForce() {
			d.log.Info("reshare_setup", "already in progress", "restart", "NOT AUTHORIZED")
			d.state.Unlock()
			return nil, errors.New("reshare already in progress; use --force")
		}
		d.log.Info("reshare_setup", "already_in_progress", "restart", "reshare")
		d.receiver.stop()
		d.receiver = nil
	}

	receiver, err := newSetupReceiver(d.log, d.opts.clock, d.privGateway.ProtocolClient, in.GetInfo())
	if err != nil {
		d.log.Error("setup", "fail", "err", err)
		d.state.Unlock()
		return nil, err
	}
	d.receiver = receiver
	defer func(r *setupReceiver) {
		d.state.Lock()
		r.stop()
		// only set to nil if the given receiver here is the same as the current
		// one, i.e. there has not been a more recent resharing comand issued in
		// between
		if d.receiver == r {
			d.receiver = nil
		}
		d.state.Unlock()
	}(d.receiver)
	d.state.Unlock()
	// send public key to leader
	id := d.priv.Public.ToProto()
	prep := &drand.SignalDKGPacket{
		Node:              id,
		SecretProof:       in.GetInfo().GetSecret(),
		PreviousGroupHash: oldHash,
	}

	// we wait only a certain amount of time for the prepare phase
	nc, cancel := context.WithTimeout(context.Background(), MaxWaitPrepareDKG)
	defer cancel()

	d.log.Info("setup_reshare", "signaling_key_to_leader")
	err = d.privGateway.ProtocolClient.SignalDKGParticipant(nc, lpeer, prep)
	if err != nil {
		d.log.Error("setup_reshare", "failed to signal key to leader", "err", err)
		return nil, fmt.Errorf("drand: err when signaling key to leader: %s", err)
	}

	newGroup, dkgTimeout, err := d.receiver.WaitDKGInfo(nc)
	if err != nil {
		d.log.Error("setup_reshare", "failed to receive dkg info", "err", err)
		return nil, err
	}

	// some assertions that should be true but never too safe
	if err := d.validateGroupTransition(oldGroup, newGroup); err != nil {
		return nil, err
	}

	node := newGroup.Find(d.priv.Public)
	if node == nil {
		// It is ok to not have our key found in the new group since we may just
		// be a node that is leaving the network, but leaving gracefully, by
		// still participating in the resharing.
		d.log.Info("setup_reshare", "not_found_in_new_group")
	} else {
		d.state.Lock()
		d.index = int(node.Index)
		d.state.Unlock()
		d.log.Info("setup_reshare", "participate_newgroup", "index", node.Index)
	}

	// run the dkg !
	finalGroup, err := d.runResharing(false, oldGroup, newGroup, dkgTimeout)
	if err != nil {
		d.log.Error("setup_reshare", "failed to run resharing", "err", err)
		return nil, err
	}
	return finalGroup.ToProto(), nil
}

func (d *Drand) validateGroupTransition(oldGroup, newGroup *key.Group) error {
	if oldGroup.GenesisTime != newGroup.GenesisTime {
		d.log.Error("setup_reshare", "invalid genesis time in received group")
		return errors.New("control: old and new group have different genesis time")
	}

	if oldGroup.Period != newGroup.Period {
		d.log.Error("setup_reshare", "invalid period time in received group")
		return errors.New("control: old and new group have different period - unsupported feature at the moment")
	}

	if !bytes.Equal(oldGroup.GetGenesisSeed(), newGroup.GetGenesisSeed()) {
		d.log.Error("setup_reshare", "invalid genesis seed in received group")
		return errors.New("control: old and new group have different genesis seed")
	}
	now := d.opts.clock.Now().Unix()
	if newGroup.TransitionTime < now {
		d.log.Error("setup_reshare", "invalid_transition", "given", newGroup.TransitionTime, "now", now)
		return errors.New("control: new group with transition time in the past")
	}
	return nil
}

func (d *Drand) extractGroup(old *drand.GroupInfo) (oldGroup *key.Group, err error) {
	d.state.Lock()
	if oldGroup, err = extractGroup(old); err != nil {
		// try to get the current group
		if d.group == nil {
			d.state.Unlock()
			return nil, errors.New("drand: can't init-reshare if no old group provided")
		}
		d.log.With("module", "control").Debug("init_reshare", "using_stored_group")
		oldGroup = d.group
		err = nil
	}
	d.state.Unlock()
	return
}

// InitReshare receives information about the old and new group from which to
// operate the resharing protocol.
func (d *Drand) InitReshare(c context.Context, in *drand.InitResharePacket) (*drand.GroupPacket, error) {
	oldGroup, err := d.extractGroup(in.Old)
	if err != nil {
		return nil, err
	}

	if !in.GetInfo().GetLeader() {
		d.log.Info("init_reshare", "begin", "leader", false)
		return d.setupAutomaticResharing(c, oldGroup, in)
	}

	d.log.Info("init_reshare", "begin", "leader", true, "time", d.opts.clock.Now())

	newSetup := func(d *Drand) (*setupManager, error) {
		return newReshareSetup(d.log, d.opts.clock, d.priv.Public, oldGroup, in)
	}

	newGroup, err := d.leaderRunSetup(newSetup)
	if err != nil {
		d.log.Error("init_reshare", "leader setup", "err", err)
		return nil, fmt.Errorf("drand: invalid setup configuration: %s", err)
	}
	if d.setupCB != nil {
		// XXX Currently a bit hacky - we should split the control plane and the
		// gRPC interface and give that callback as argument
		d.setupCB(newGroup)
	}
	// some assertions that should always be true but never too safe
	if oldGroup.GenesisTime != newGroup.GenesisTime {
		return nil, errors.New("control: old and new group have different genesis time")
	}
	if oldGroup.GenesisTime > d.opts.clock.Now().Unix() {
		return nil, errors.New("control: genesis time is in the future")
	}
	if oldGroup.Period != newGroup.Period {
		return nil, errors.New("control: old and new group have different period - unsupported feature at the moment")
	}
	if newGroup.TransitionTime < d.opts.clock.Now().Unix() {
		return nil, errors.New("control: group with transition time in the past")
	}
	if !bytes.Equal(newGroup.GetGenesisSeed(), oldGroup.GetGenesisSeed()) {
		return nil, errors.New("control: old and new group have different genesis seed")
	}

	// send it to everyone in the group nodes
	if err := d.pushDKGInfo(oldGroup.Nodes, newGroup.Nodes,
		oldGroup.Threshold,
		newGroup,
		in.GetInfo().GetSecret(),
		in.GetInfo().GetTimeout()); err != nil {
		d.log.Error("push_group", err)
		return nil, errors.New("fail to push new group")
	}

	finalGroup, err := d.runResharing(true, oldGroup, newGroup, in.GetInfo().GetTimeout())
	if err != nil {
		return nil, err
	}
	return finalGroup.ToProto(), nil
}

// PingPong simply responds with an empty packet, proving that this drand node
// is up and alive.
func (d *Drand) PingPong(c context.Context, in *drand.Ping) (*drand.Pong, error) {
	return &drand.Pong{}, nil
}

// Share is a functionality of Control Service defined in protobuf/control that requests the private share of the drand node running locally
func (d *Drand) Share(ctx context.Context, in *drand.ShareRequest) (*drand.ShareResponse, error) {
	share, err := d.store.LoadShare()
	if err != nil {
		return nil, err
	}
	id := uint32(share.Share.I)
	buff, err := share.Share.V.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &drand.ShareResponse{Index: id, Share: buff}, nil
}

// PublicKey is a functionality of Control Service defined in protobuf/control
// that requests the long term public key of the drand node running locally
func (d *Drand) PublicKey(ctx context.Context, in *drand.PublicKeyRequest) (*drand.PublicKeyResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()
	keyPair, err := d.store.LoadKeyPair()
	if err != nil {
		return nil, err
	}
	protoKey, err := keyPair.Public.Key.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &drand.PublicKeyResponse{PubKey: protoKey}, nil
}

// PrivateKey is a functionality of Control Service defined in protobuf/control
// that requests the long term private key of the drand node running locally
func (d *Drand) PrivateKey(ctx context.Context, in *drand.PrivateKeyRequest) (*drand.PrivateKeyResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()
	keyPair, err := d.store.LoadKeyPair()
	if err != nil {
		return nil, err
	}
	protoKey, err := keyPair.Key.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &drand.PrivateKeyResponse{PriKey: protoKey}, nil
}

// GroupFile replies with the distributed key in the response
func (d *Drand) GroupFile(ctx context.Context, in *drand.GroupRequest) (*drand.GroupPacket, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.group == nil {
		return nil, errors.New("drand: no dkg group setup yet")
	}
	protoGroup := d.group.ToProto()
	return protoGroup, nil
}

// Shutdown stops the node
func (d *Drand) Shutdown(ctx context.Context, in *drand.ShutdownRequest) (*drand.ShutdownResponse, error) {
	d.Stop(ctx)
	return nil, nil
}

func extractGroup(i *drand.GroupInfo) (*key.Group, error) {
	var g = new(key.Group)
	switch x := i.Location.(type) {
	case *drand.GroupInfo_Path:
		// search group file via local filesystem path
		if err := key.Load(x.Path, g); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("control: can't allow new empty group")
	}
	if g.Threshold < vss.MinimumT(g.Len()) {
		return nil, errors.New("control: threshold of new group too low ")
	}
	return g, nil
}

func extractEntropy(i *drand.EntropyInfo) (io.Reader, bool) {
	if i == nil {
		return nil, false
	}
	r := entropy.NewScriptReader(i.Script)
	user := i.UserOnly
	return r, user
}

func (d *Drand) getPhaser(timeout uint32) *dkg.TimePhaser {
	tDuration := time.Duration(timeout) * time.Second
	if timeout == 0 {
		tDuration = DefaultDKGTimeout
	}
	return dkg.NewTimePhaserFunc(func(phase dkg.Phase) {
		d.opts.clock.Sleep(tDuration)
		d.log.Debug("phaser_finished", phase)
	})
}

func nodesContainAddr(nodes []*key.Node, addr string) bool {
	for _, n := range nodes {
		if n.Address() == addr {
			return true
		}
	}
	return false
}

// nodeUnion takes the union of two sets of nodes
func nodeUnion(a, b []*key.Node) []*key.Node {
	out := make([]*key.Node, 0, len(a))
	out = append(out, a...)
	for _, n := range b {
		if !nodesContainAddr(a, n.Address()) {
			out = append(out, n)
		}
	}
	return out
}

type pushResult struct {
	address string
	err     error
}

// pushDKGInfoPacket sets a specific DKG info packet to spcified nodes, and returns a stream of responses.
func (d *Drand) pushDKGInfoPacket(ctx context.Context, nodes []*key.Node, packet *drand.DKGInfoPacket) chan pushResult {
	results := make(chan pushResult, len(nodes))

	for _, node := range nodes {
		if node.Address() == d.priv.Public.Address() {
			continue
		}
		go func(i *key.Identity) {
			err := d.privGateway.ProtocolClient.PushDKGInfo(ctx, i, packet)
			results <- pushResult{i.Address(), err}
		}(node.Identity)
	}

	return results
}

// pushDKGInfo sends the information to run the DKG to all specified nodes. The
// call is blocking until all nodes have replied or after one minute timeouts.
func (d *Drand) pushDKGInfo(outgoing, incoming []*key.Node, previousThreshold int, group *key.Group, secret []byte, timeout uint32) error {
	// sign the group to prove you are the leader
	signature, err := key.DKGAuthScheme.Sign(d.priv.Key, group.Hash())
	if err != nil {
		d.log.Error("setup", "leader", "group_signature", err)
		return fmt.Errorf("drand: error signing group: %w", err)
	}
	packet := &drand.DKGInfoPacket{
		NewGroup:    group.ToProto(),
		SecretProof: secret,
		DkgTimeout:  timeout,
		Signature:   signature,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	newThreshold := group.Threshold
	if nodesContainAddr(outgoing, d.priv.Public.Address()) {
		previousThreshold--
	}
	if nodesContainAddr(incoming, d.priv.Public.Address()) {
		newThreshold--
	}
	to := nodeUnion(outgoing, incoming)

	results := d.pushDKGInfoPacket(ctx, to, packet)

	total := len(to) - 1
	for total > 0 {
		select {
		case ok := <-results:
			total--
			if ok.err != nil {
				d.log.Error("push_dkg", "failed to push", "to", ok.address, "err", ok.err)
				continue
			}
			d.log.Debug("push_dkg", "sending_group", "success_to", ok.address, "left", total)
			if nodesContainAddr(outgoing, ok.address) {
				previousThreshold--
			}
			if nodesContainAddr(incoming, ok.address) {
				newThreshold--
			}
		case <-d.opts.clock.After(time.Minute):
			if previousThreshold <= 0 && newThreshold <= 0 {
				d.log.Info("push_dkg", "sending_group", "status", "enough succeeded", "missed", total)
				return nil
			}
			d.log.Warn("push_dkg", "sending_group", "status", "timeout")
			return errors.New("push group timeout")
		}
	}
	if previousThreshold > 0 || newThreshold > 0 {
		d.log.Info("push_dkg", "sending_group", "status", "not enough succeeded", "prev", previousThreshold, "new", newThreshold)
		return errors.New("push group failure")
	}
	d.log.Info("push_dkg", "sending_group", "status", "all succeeded")
	return nil
}

func getNonce(g *key.Group) []byte {
	h := sha256.New()
	if g.TransitionTime != 0 {
		_ = binary.Write(h, binary.BigEndian, g.TransitionTime)
	} else {
		_ = binary.Write(h, binary.BigEndian, g.GenesisTime)
	}
	return h.Sum(nil)
}

// StartFollowChain syncs up with a chain from other nodes
func (d *Drand) StartFollowChain(req *drand.StartFollowRequest, stream drand.Control_StartFollowChainServer) error {
	// TODO replace via a more independent chain manager that manages the
	// transition from following -> participating
	d.state.Lock()
	if d.syncerCancel != nil {
		d.state.Unlock()
		return errors.New("syncing is already in progress")
	}
	// context given to the syncer
	// NOTE: this means that if the client quits the requests, the syncing
	// context will signal and it will stop. If we want the following to
	// continue nevertheless we can use the next line by using a new context.
	// ctx, cancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(stream.Context())
	d.syncerCancel = cancel
	d.state.Unlock()
	defer func() {
		d.state.Lock()
		if d.syncerCancel != nil {
			// it can be nil when we recreate a new beacon we cancel it
			// see drand.go:newBeacon()
			d.syncerCancel()
		}
		d.syncerCancel = nil
		d.state.Unlock()
	}()

	addr := net.RemoteAddress(stream.Context())
	peers := make([]net.Peer, 0, len(req.GetNodes()))
	for _, addr := range req.GetNodes() {
		// XXX add TLS disable later
		peers = append(peers, net.CreatePeer(addr, req.GetIsTls()))
	}
	info, err := chainInfoFromPeers(stream.Context(), d.privGateway, peers, d.log)
	if err != nil {
		return err
	}
	d.log.Debug("start_follow_chain", "fetched chain info", "hash", fmt.Sprintf("%x", info.Hash()))

	hashStr := req.GetInfoHash()
	hash, err := hex.DecodeString(hashStr)
	if err != nil {
		return fmt.Errorf("invalid hash info hex: %v", err)
	}
	if !bytes.Equal(info.Hash(), hash) {
		return errors.New("invalid chain info hash")
	}

	store, err := d.createBoltStore()
	if err != nil {
		d.log.Error("start_follow_chain", "unable to create store", "err", err)
		return fmt.Errorf("unable to create store: %s", err)
	}

	// TODO find a better place to put that
	if err := store.Put(chain.GenesisBeacon(info)); err != nil {
		d.log.Error("start_follow_chain", "unable to insert genesis block", "err", err)
		store.Close()
		return fmt.Errorf("unable to insert genesis block: %s", err)
	}
	// register callback to notify client of progress
	cbStore := beacon.NewCallbackStore(store)
	defer cbStore.Close()
	syncer := beacon.NewSyncer(d.log, cbStore, info, d.privGateway)
	cb, done := sendProgressCallback(stream, req.GetUpTo(), info, d.opts.clock, d.log)
	cbStore.AddCallback(addr, cb)
	defer cbStore.RemoveCallback(addr)
	if err := syncer.Follow(ctx, req.GetUpTo(), peers); err != nil {
		d.log.Error("start_follow_chain", "syncer_stopped", "err", err, "leaving_sync")
		return err
	}
	// wait for all the callbacks to be called and progress sent before returning
	if req.GetUpTo() > 0 {
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return ctx.Err()
}

// chainInfoFromPeers attempts to fetch chain info from one of the passed peers.
func chainInfoFromPeers(ctx context.Context, privGateway *net.PrivateGateway, peers []net.Peer, l log.Logger) (*chain.Info, error) {
	var info *chain.Info
	for _, peer := range peers {
		ci, err := privGateway.ChainInfo(ctx, peer, new(drand.ChainInfoRequest))
		if err != nil {
			l.Debug("start_follow_chain", "error getting chain info", "from", peer.Address(), "err", err)
			continue
		}
		info, err = chain.InfoFromProto(ci)
		if err != nil {
			l.Debug("start_follow_chain", "invalid chain info", "from", peer.Address(), "err", err)
			continue
		}
	}
	if info == nil {
		return nil, errors.New("unable to get a chain info successfully")
	}
	return info, nil
}

// sendProgressCallback returns a function that sends FollowProgress on the
// passed stream. It also returns a channel that closes when the callback is
// called with a beacon whose round matches the passed upTo value.
func sendProgressCallback(
	stream drand.Control_StartFollowChainServer,
	upTo uint64,
	info *chain.Info,
	clk clock.Clock,
	l log.Logger,
) (cb func(b *chain.Beacon), done chan struct{}) {
	done = make(chan struct{})
	cb = func(b *chain.Beacon) {
		err := stream.Send(&drand.FollowProgress{
			Current: b.Round,
			Target:  chain.CurrentRound(clk.Now().Unix(), info.Period, info.GenesisTime),
		})
		if err != nil {
			l.Error("send_progress_callback", "sending_progress", "err", err)
		}
		if upTo > 0 && b.Round == upTo {
			close(done)
		}
	}
	return
}

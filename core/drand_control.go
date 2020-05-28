package core

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/drand/drand/entropy"
	"github.com/drand/drand/key"
	dnet "github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	control "github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share/dkg"
	vss "github.com/drand/kyber/share/vss/pedersen"
)

// InitDKG take a InitDKGPacket, extracts the informations needed and wait for the
// DKG protocol to finish. If the request specifies this node is a leader, it
// starts the DKG protocol.
func (d *Drand) InitDKG(c context.Context, in *control.InitDKGPacket) (*control.GroupPacket, error) {
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
	newSetup := func() (*setupManager, error) {
		return newDKGSetup(d.log, d.opts.clock, d.priv.Public, in.GetBeaconPeriod(), in.GetInfo())
	}

	// expect the group
	group, err := d.leaderRunSetup(in.GetInfo(), newSetup)
	if err != nil {
		return nil, fmt.Errorf("drand: invalid setup configuration: %s", err)
	}

	protoGroup := group.ToProto()
	packet := &drand.DKGInfoPacket{
		NewGroup:    protoGroup,
		SecretProof: in.GetInfo().GetSecret(),
		DkgTimeout:  in.GetInfo().GetTimeout(),
	}
	// send it to everyone in the group nodes
	nodes := group.Nodes
	if err := d.pushDKGInfo(nodes, packet); err != nil {
		return nil, err
	}
	finalGroup, err := d.runDKG(true, group, in.GetInfo().GetTimeout(), in.GetEntropy())
	if err != nil {
		return nil, err
	}
	return finalGroup.ToProto(), nil
}

func (d *Drand) leaderRunSetup(in *control.SetupInfoPacket, newSetup func() (*setupManager, error)) (*key.Group, error) {
	// setup the manager
	d.state.Lock()
	if d.manager != nil {
		d.log.Info("reshare", "already_in_progress", "restart", "reshare")
		d.manager.StopPreemptively()
	}
	manager, err := newSetup()
	if err != nil {
		return nil, fmt.Errorf("drand: invalid setup configuration: %s", err)
	}
	go manager.run()

	d.manager = manager
	d.state.Unlock()
	defer func() {
		d.state.Lock()
		// set back manager to nil afterwards to be able to run a new setup
		d.manager = nil
		d.state.Unlock()
	}()

	// wait to receive the keys & send them to the other nodes
	var group *key.Group
	select {
	case group = <-d.manager.WaitGroup():
		var addr []string
		for _, k := range group.Nodes {
			addr = append(addr, k.Address())
		}
		d.log.Debug("init_dkg", "setup_phase", "keys_received", "["+strings.Join(addr, "-")+"]")
	case <-time.After(MaxWaitPrepareDKG):
		d.log.Debug("init_dkg", "time_out")
		manager.StopPreemptively()
		return nil, errors.New("time outs: no key received")
	}
	return group, nil
}

// runDKG setups the proper structures and protocol to run the DKG and waits
// until it finishes. If leader is true, this node sends the first packet.
func (d *Drand) runDKG(leader bool, group *key.Group, timeout uint32, entropy *control.EntropyInfo) (*key.Group, error) {

	reader, user := extractEntropy(entropy)
	dkgConfig := dkg.DkgConfig{
		Suite:          key.KeyGroup.(dkg.Suite),
		NewNodes:       group.DKGNodes(),
		Longterm:       d.priv.Key,
		Reader:         reader,
		UserReaderOnly: user,
		FastSync:       true,
		Threshold:      group.Threshold,
	}
	phaser, err := d.getPhaser(timeout)
	if err != nil {
		return nil, fmt.Errorf("drand: invalid timeout: %s", err)
	}
	board := newBoard(d.log, d.privGateway.ProtocolClient, d.priv.Public, group)
	protoConf := &dkg.Config{
		DkgConfig: dkgConfig,
		Auth:      key.AuthScheme,
	}
	dkgProto, err := dkg.NewProtocol(protoConf, board, phaser)
	if err != nil {
		return nil, err
	}

	d.state.Lock()
	d.dkgInfo = &dkgInfo{
		target: group,
		board:  board,
		phaser: phaser,
		conf:   protoConf,
		proto:  dkgProto,
	}
	if leader {
		d.dkgInfo.started = true
	}
	d.state.Unlock()

	d.log.Info("init_dkg", "start_dkg")
	if leader {
		// phaser will kick off the first phase for every other nodes so
		// nodes will send their deals
		go phaser.Start()
	}
	finalGroup, err := d.WaitDKG()
	if err != nil {
		return nil, fmt.Errorf("drand: err during DKG: %v", err)
	}
	d.log.Info("init_dkg", "dkg_done", "starting_beacon_time", finalGroup.GenesisTime, "now", d.opts.clock.Now().Unix())
	// beacon will start at the genesis time specified
	go d.StartBeacon(false)
	return finalGroup, nil
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

	dkgConfig := dkg.DkgConfig{
		Suite:        key.KeyGroup.(dkg.Suite),
		NewNodes:     newGroup.DKGNodes(),
		OldNodes:     oldGroup.DKGNodes(),
		Longterm:     d.priv.Key,
		Threshold:    newGroup.Threshold,
		OldThreshold: oldGroup.Threshold,
		FastSync:     true,
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
			dkgConfig.Share = &dkgShare
		} else {
			// we are a new node, we want to make sure we reshare from the old
			// group public key
			dkgConfig.PublicCoeffs = oldGroup.PublicKey.Coefficients
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	board := newReshareBoard(d.log, d.privGateway.ProtocolClient, d.priv.Public, oldGroup, newGroup)
	protoConf := &dkg.Config{
		DkgConfig: dkgConfig,
		Auth:      key.AuthScheme,
	}
	phaser, err := d.getPhaser(timeout)
	if err != nil {
		return nil, fmt.Errorf("drand: invalid timeout: %s", err)
	}

	dkgProto, err := dkg.NewProtocol(protoConf, board, phaser)
	if err != nil {
		return nil, err
	}
	info := &dkgInfo{
		target: newGroup,
		board:  board,
		phaser: phaser,
		conf:   protoConf,
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

	d.log.Info("init_dkg", "wait_dkg_end")
	finalGroup, err := d.WaitDKG()
	if err != nil {
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
func (d *Drand) setupAutomaticDKG(c context.Context, in *control.InitDKGPacket) (*control.GroupPacket, error) {
	d.log.Info("init_dkg", "begin", "leader", false)
	// determine the leader's address
	laddr := in.GetInfo().GetLeaderAddress()
	lpeer := dnet.CreatePeer(laddr, in.GetInfo().GetLeaderTls())
	d.state.Lock()
	if d.receiver != nil {
		d.log.Info("dkg_setup", "already_in_progress", "restart", "dkg")
		d.receiver.stop()
	}
	receiver := newSetupReceiver(d.log, in.GetInfo())
	d.receiver = receiver
	d.state.Unlock()

	defer func() {
		d.state.Lock()
		d.receiver.stop()
		d.receiver = nil
		d.state.Unlock()
	}()
	// send public key to leader
	pubKey, _ := d.priv.Public.Key.MarshalBinary()
	id := &drand.Identity{
		Address: d.priv.Public.Address(),
		Key:     pubKey,
		Tls:     d.priv.Public.IsTLS(),
	}
	prep := &drand.SignalDKGPacket{
		Node:        id,
		SecretProof: in.GetInfo().GetSecret(),
	}

	d.log.Debug("init_dkg", "send_key", "leader", lpeer.Address())
	err := d.privGateway.ProtocolClient.SignalDKGParticipant(context.Background(), lpeer, prep)
	if err != nil {
		return nil, fmt.Errorf("drand: err when receiving group: %s", err)
	}

	d.log.Debug("init_dkg", "wait_group")
	var dkgInfo *drand.DKGInfoPacket
	select {
	case dkgInfo = <-d.receiver.WaitDKGInfo():
		if dkgInfo == nil {
			d.log.Debug("init_dkg", "wait_group", "cancelled", "nil_group")
			return nil, errors.New("cancelled operation")
		}
		d.log.Debug("init_dkg", "received_group")
	case <-d.opts.clock.After(MaxWaitPrepareDKG):
		d.log.Error("init_dkg", "wait_group", "err", "timeout")
		return nil, errors.New("wait_group timeouts from coordinator")
	}

	// verify things are all in order
	group, err := key.GroupFromProto(dkgInfo.NewGroup)
	if err != nil {
		return nil, fmt.Errorf("group from leader invalid: %s", err)
	}
	now := d.opts.clock.Now().Unix()
	if group.GenesisTime < now {
		d.log.Error("genesis", "invalid", "given", group.GenesisTime, "now", d.opts.clock.Now().Unix())
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
	finalGroup, err := d.runDKG(false, group, dkgInfo.GetDkgTimeout(), in.GetEntropy())
	if err != nil {
		return nil, err
	}
	return finalGroup.ToProto(), nil
}

// similar to setupAutomaticDKG but with additional verification and information
// w.r.t. to the previous group
func (d *Drand) setupAutomaticResharing(c context.Context, oldGroup *key.Group, in *control.InitResharePacket) (*control.GroupPacket, error) {
	oldHash := oldGroup.Hash()
	// determine the leader's address
	laddr := in.GetInfo().GetLeaderAddress()
	lpeer := dnet.CreatePeer(laddr, in.GetInfo().GetLeaderTls())
	d.state.Lock()
	if d.receiver != nil {
		d.log.Info("reshare_setup", "already_in_progress", "restart", "reshare")
		d.receiver.stop()
	}

	receiver := newSetupReceiver(d.log, in.GetInfo())
	d.receiver = receiver
	d.state.Unlock()

	defer func() {
		d.state.Lock()
		d.receiver.stop()
		d.receiver = nil
		d.state.Unlock()
	}()
	// send public key to leader
	pubKey, _ := d.priv.Public.Key.MarshalBinary()
	id := &drand.Identity{
		Address: d.priv.Public.Address(),
		Key:     pubKey,
		Tls:     d.priv.Public.IsTLS(),
	}
	prep := &drand.SignalDKGPacket{
		Node:              id,
		SecretProof:       in.GetInfo().GetSecret(),
		PreviousGroupHash: oldHash,
	}

	// we wait only a certain amount of time for the prepare phase
	nc, cancel := context.WithTimeout(c, MaxWaitPrepareDKG)
	defer cancel()

	d.log.Info("setup_reshare", "signalling_key_to_leader")
	err := d.privGateway.ProtocolClient.SignalDKGParticipant(nc, lpeer, prep)
	if err != nil {
		return nil, fmt.Errorf("drand: err when receiving group: %s", err)
	}

	var dkgInfo *drand.DKGInfoPacket
	select {
	case dkgInfo = <-d.receiver.WaitDKGInfo():
		d.log.Debug("setup_reshare", "received_group")
	case <-d.opts.clock.After(MaxWaitPrepareDKG):
		d.log.Error("setup_reshare", "prepare_dkg_timeout")
		return nil, errors.New("prepare_dkg_timeout")
	}

	// verify things are all in order
	newGroup, err := key.GroupFromProto(dkgInfo.NewGroup)
	if err != nil {
		return nil, fmt.Errorf("group from leader invalid: %s", err)
	}

	// some assertions that should be true but never too safe
	if oldGroup.GenesisTime != newGroup.GenesisTime {
		return nil, errors.New("control: old and new group have different genesis time")
	}

	if oldGroup.Period != newGroup.Period {
		return nil, errors.New("control: old and new group have different period - unsupported feature at the moment")
	}

	if !bytes.Equal(oldGroup.GetGenesisSeed(), newGroup.GetGenesisSeed()) {
		return nil, errors.New("control: old and new group have different genesis seed")
	}
	now := d.opts.clock.Now().Unix()
	if newGroup.TransitionTime < now {
		d.log.Error("setup_reshare", "invalid_transition", "given", newGroup.TransitionTime, "now", now)
		return nil, errors.New("control: new group with transition time in the past")
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
	finalGroup, err := d.runResharing(false, oldGroup, newGroup, dkgInfo.GetDkgTimeout())
	if err != nil {
		return nil, err
	}
	return finalGroup.ToProto(), nil
}

// InitReshare receives information about the old and new group from which to
// operate the resharing protocol.
func (d *Drand) InitReshare(c context.Context, in *control.InitResharePacket) (*control.GroupPacket, error) {
	var oldGroup *key.Group
	var err error

	d.state.Lock()
	if oldGroup, err = extractGroup(in.Old); err != nil {
		// try to get the current group
		if d.group == nil {
			d.state.Unlock()
			return nil, errors.New("drand: can't init-reshare if no old group provided")
		}
		d.log.With("module", "control").Debug("init_reshare", "using_stored_group")
		oldGroup = d.group
	}
	d.state.Unlock()

	if !in.GetInfo().GetLeader() {
		d.log.Info("init_reshare", "begin", "leader", false)
		return d.setupAutomaticResharing(c, oldGroup, in)
	}
	d.log.Info("init_reshare", "begin", "leader", true, "time", d.opts.clock.Now())

	newSetup := func() (*setupManager, error) {
		return newReshareSetup(d.log, d.opts.clock, d.priv.Public, oldGroup, in)
	}

	newGroup, err := d.leaderRunSetup(in.GetInfo(), newSetup)
	if err != nil {
		return nil, fmt.Errorf("drand: invalid setup configuration: %s", err)
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

	// send to all previous nodes + new nodes
	var seen = make(map[string]bool)
	seen[d.priv.Public.Address()] = true
	var to []*key.Node
	for _, node := range oldGroup.Nodes {
		if seen[node.Address()] {
			continue
		}
		to = append(to, node)
	}
	for _, node := range newGroup.Nodes {
		if seen[node.Address()] {
			continue
		}
		to = append(to, node)
	}

	protoGroup := newGroup.ToProto()
	packet := &drand.DKGInfoPacket{
		SecretProof: in.GetInfo().GetSecret(),
		DkgTimeout:  in.GetInfo().GetTimeout(),
		NewGroup:    protoGroup,
	}
	// send it to everyone in the group nodes
	if err := d.pushDKGInfo(to, packet); err != nil {
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
func (d *Drand) PingPong(c context.Context, in *control.Ping) (*control.Pong, error) {
	return &control.Pong{}, nil
}

// Share is a functionality of Control Service defined in protobuf/control that requests the private share of the drand node running locally
func (d *Drand) Share(ctx context.Context, in *control.ShareRequest) (*control.ShareResponse, error) {
	share, err := d.store.LoadShare()
	if err != nil {
		return nil, err
	}
	id := uint32(share.Share.I)
	buff, err := share.Share.V.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &control.ShareResponse{Index: id, Share: buff}, nil
}

// PublicKey is a functionality of Control Service defined in protobuf/control that requests the long term public key of the drand node running locally
func (d *Drand) PublicKey(ctx context.Context, in *control.PublicKeyRequest) (*control.PublicKeyResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()
	key, err := d.store.LoadKeyPair()
	if err != nil {
		return nil, err
	}
	protoKey, err := key.Public.Key.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &control.PublicKeyResponse{PubKey: protoKey}, nil
}

// PrivateKey is a functionality of Control Service defined in protobuf/control that requests the long term private key of the drand node running locally
func (d *Drand) PrivateKey(ctx context.Context, in *control.PrivateKeyRequest) (*control.PrivateKeyResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()
	key, err := d.store.LoadKeyPair()
	if err != nil {
		return nil, err
	}
	protoKey, err := key.Key.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &control.PrivateKeyResponse{PriKey: protoKey}, nil
}

// GroupFile replies with the distributed key in the response
func (d *Drand) GroupFile(ctx context.Context, in *control.GroupRequest) (*control.GroupPacket, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.group == nil {
		return nil, errors.New("drand: no dkg group setup yet")
	}
	protoGroup := d.group.ToProto()
	return protoGroup, nil
}

// Shutdown stops the node
func (d *Drand) Shutdown(ctx context.Context, in *control.ShutdownRequest) (*control.ShutdownResponse, error) {
	d.Stop(ctx)
	return nil, nil
}

func extractGroup(i *control.GroupInfo) (*key.Group, error) {
	var g = new(key.Group)
	switch x := i.Location.(type) {
	case *control.GroupInfo_Path:
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

func extractEntropy(i *control.EntropyInfo) (io.Reader, bool) {
	if i == nil {
		return nil, false
	}
	r := entropy.NewScriptReader(i.Script)
	user := i.UserOnly
	return r, user
}

func (d *Drand) getPhaser(timeout uint32) (*dkg.TimePhaser, error) {
	tDuration := time.Duration(timeout) * time.Second
	if timeout == 0 {
		tDuration = DefaultDKGTimeout
	}
	return dkg.NewTimePhaserFunc(func(phase dkg.Phase) {
		d.opts.clock.Sleep(tDuration)
		d.log.Debug("phaser_finished", phase)
	}), nil
}

// pushDKGInfo sends the information to run the DKG to all specified nodes. The
// call is blocking until all nodes have replied or after one minute timeouts.
func (d *Drand) pushDKGInfo(to []*key.Node, packet *drand.DKGInfoPacket) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var tooLate = make(chan bool, 1)
	var success = make(chan string, len(to))
	go func() {
		<-d.opts.clock.After(time.Minute)
		tooLate <- true
	}()
	for _, node := range to {
		if node.Address() == d.priv.Public.Address() {
			continue
		}
		go func(i *key.Identity) {
			err := d.privGateway.ProtocolClient.PushDKGInfo(ctx, i, packet)
			if err != nil {
				d.log.Error("push_dkg", "failed to push", "to", i.Address(), "err", err)
			} else {
				success <- i.Address()
			}
		}(node.Identity)
	}
	exp := len(to) - 1
	got := 0
	for got < exp {
		select {
		case ok := <-success:
			d.log.Debug("push_dkg", "sending_group", "success_to", ok)
			got++
		case <-tooLate:
			return errors.New("push group timeout")
		}
	}
	d.log.Info("push_dkg", "sending_group", "status", "done")
	cancel()
	return nil
}

package core

import (
	"bytes"
	"context"
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
	clock "github.com/jonboulle/clockwork"
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
		return d.setupAutomaticDKG(c, in)
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

	// we start the dkg at some point in the future - every node will start a
	// this time
	startTime := getDKGStartTime(h.opts.clock, in.GetInfo())
	protoGroup := groupToProto(group)
	packet := &drand.DKGInfoPacket{
		NewGroup:     protoGroup,
		SecretProof:  in.GetInfo().GetSecret(),
		DkgStartTime: startTime,
	}
	// send it to everyone in the group nodes
	nodes := group.Nodes
	if err := d.pushDKGInfo(nodes, packet); err != nil {
		return nil, err
	}

	finalGroup, err := d.runDKG(startTime, true, group, in.GetInfo().GetTimeout(), in.GetEntropy())
	if err != nil {
		return nil, err
	}
	return groupToProto(finalGroup), nil
}

func (d *Drand) leaderRunSetup(in *control.SetupInfoPacket, newSetup func() (*setupManager, error)) (*key.Group, error) {
	// setup the manager
	d.state.Lock()
	if d.manager != nil {
		return nil, errors.New("drand: setup dkg already in progress")
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
		for _, k := range group.Identities() {
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

func (d *Drand) runDKG(startTime int64, leader bool, group *key.Group, timeout uint32, entropy *control.EntropyInfo) (*key.Group, error) {

	reader, user := extractEntropy(entropy)
	dkgConfig := &dkg.DkgConfig{
		Suite:          key.KeyGroup.(dkg.Suite),
		NewNodes:       group,
		Key:            d.priv,
		Reader:         reader,
		UserReaderOnly: user,
		FastSync:       true,
	}
	phaser, err := d.getPhaser(timeout)
	if err != nil {
		return nil, fmt.Errorf("drand: invalid timeout: %s", err)
	}
	board := newBoard(d.log, d.control)
	protoConf := &dkg.Config{
		DkgConfig:  dkgConfig,
		AuthScheme: key.AuthScheme,
	}
	dkgProto, err := dkg.NewProto(protoConf, board, phaser)
	if err != nil {
		return err
	}

	d.state.Lock()
	d.dkgInfo = &dkgInfo{
		target: group,
		board:  board,
		phaser: phaser,
		conf:   protoConf,
		proto:  dkgProto,
	}
	d.state.Unlock()

	d.waitForDKGStartTime(startTime)
	d.log.Info("init_dkg", "start_dkg")
	// phaser will kick off the first phase so nodes will send their deals
	go phaser.Start()
	finalGroup, err := d.WaitDKG()
	if err != nil {
		return nil, fmt.Errorf("drand: err during DKG: %v", err)
	}
	d.log.Info("init_dkg", "dkg_done")
	// beacon will start at the genesis time specified
	d.StartBeacon(false)
	return finalGroup, nil
}

func (d *Drand) runResharing(startTime int64, leader bool, oldGroup, newGroup *key.Group, timeout string) (*key.Group, error) {
	oldIdx, oldPresent := oldGroup.Index(d.priv.Public)
	_, newPresent := newGroup.Index(d.priv.Public)

	dkgConfig := &dkg.DkgConfig{
		Suite:    key.KeyGroup.(dkg.Suite),
		NewNodes: newGroup,
		OldNodes: oldGroup,
		Key:      d.priv,
		Clock:    d.opts.clock,
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
			dkgConfig.Share = d.share
		}
	}()
	if err != nil {
		return nil, err
	}
	board := newBoard(d.log, d.gateway)
	protoConf := &dkg.Config{
		DkgConfig:  dkgConfig,
		AuthScheme: key.AuthScheme,
	}
	phaser, err = d.getPhaser(timeout)
	if err != nil {
		return nil, fmt.Errorf("drand: invalid timeout: %s", err)
	}

	dkgProto, err := dkg.NewProto(protoConf, board, phaser)
	if err != nil {
		return err
	}
	info := &dkgInfo{
		target: newGroup,
		board:  board,
		phaser: phaser,
		config: protoConf,
		proto:  dkgProto,
	}
	d.state.Lock()
	d.dkgInfo = info
	d.state.Unlock()

	d.waitForDKGStartTime(startTime)

	d.log.Info("init_dkg", "wait_dkg_end")
	finalGroup, err := d.WaitDKG(dkgConfig)
	if err != nil {
		return nil, fmt.Errorf("drand: err during DKG: %v", err)
	}
	d.log.Info("dkg_reshare", "finished")
	// runs the transition of the beacon
	go d.transition(oldGroup, oldPresent, newPresent)
	return finalGroup, nil
}

// This method sends the public key to the denoted leader address and then waits
// to receive the group file. After receiving it, it starts the DKG process in
// "waiting" mode, waiting for the leader to send the first packet.
func (d *Drand) setupAutomaticDKG(c context.Context, in *control.InitDKGPacket) (*control.GroupPacket, error) {
	d.log.Info("init_dkg", "begin", "leader", false)
	n, thr, dkgTimeout, err := validInitPacket(in.GetInfo())
	if err != nil {
		return nil, err
	}
	// determine the leader's address
	laddr := in.GetInfo().GetLeaderAddress()
	lpeer := dnet.CreatePeer(laddr, in.GetInfo().GetLeaderTls())
	d.state.Lock()
	if d.receiver != nil {
		d.state.Unlock()
		return nil, errors.New("drand: already waiting for an automatic setup")
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
	key, _ := d.priv.Public.Key.MarshalBinary()
	id := &drand.Identity{
		Address: d.priv.Public.Address(),
		Key:     key,
		Tls:     d.priv.Public.IsTLS(),
	}
	prep := &drand.PrepareDKGPacket{
		Node:        id,
		Expected:    uint32(n),
		Threshold:   uint32(thr),
		DkgTimeout:  uint64(dkgTimeout.Seconds()),
		SecretProof: in.GetInfo().GetSecret(),
	}

	d.log.Debug("init_dkg", "send_key", "leader", lpeer.Address())
	err = d.gateway.ProtocolClient.PrepareDKGGroup(context.Background(), lpeer, prep)
	if err != nil {
		return nil, fmt.Errorf("drand: err when receiving group: %s", err)
	}

	d.log.Debug("init_dkg", "wait_group")
	var dkgInfo *drand.DKGInfoPacket
	select {
	case dkgInfo = <-d.receiver.WaitDKGInfo():
		d.log.Debug("init_dkg", "received_group")
	case <-d.opts.clock.After(MaxWaitPrepareDKG):
		d.log.Error("init_dkg", "wait_group", "timeout")
		return nil, errors.New("wait_group timeouts from coordinator")
	}

	// verify things are all in order
	group, err := ProtoToGroup(dkgInfo.NewGroup)
	if err != nil {
		return nil, fmt.Errorf("group from leader invalid: %s", err)
	}
	now := d.opts.clock.Now().Unix()
	if group.GenesisTime < now {
		d.log.Error("genesis", "invalid", "given", group.GenesisTime, "now", d.opts.clock.Now().Unix())
		return nil, errors.New("control: group with genesis time in the past")
	}
	if dkgInfo.StartTime < now {
		d.log.Error("init_dkg", "invaid_start_time", "given", dkgInfo.StartTime, "now", now)
		return nil, errors.New("control: invalid start time")
	}

	index, found := group.Index(d.priv.Public)
	if !found {
		fmt.Println("priv is ", d.priv.Public.String(), " but received group is ", group.String())
		return nil, errors.New("drand: public key not found in group")
	}
	d.state.Lock()
	d.index = index
	d.state.Unlock()

	// run the dkg !
	finalGroup, err := d.runDKG(dkgInfo.StartTime, false, group, in.GetInfo().GetTimeout(), in.GetEntropy())
	if err != nil {
		return nil, err
	}
	return groupToProto(finalGroup), nil
}

// similar to setupAutomaticDKG but with additional verification and information
// w.r.t. to the previous group
func (d *Drand) setupAutomaticResharing(c context.Context, oldGroup *key.Group, in *control.InitResharePacket) (*control.GroupPacket, error) {
	n, thr, dkgTimeout, err := validInitPacket(in.GetInfo())
	if err != nil {
		return nil, err
	}
	oldHash, err := oldGroup.Hash()
	if err != nil {
		return nil, err
	}
	// determine the leader's address
	laddr := in.GetInfo().GetLeaderAddress()
	lpeer := dnet.CreatePeer(laddr, in.GetInfo().GetLeaderTls())
	d.state.Lock()
	if d.receiver != nil {
		d.state.Unlock()
		return nil, errors.New("drand: already waiting for an automatic setup")
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
	key, _ := d.priv.Public.Key.MarshalBinary()
	id := &drand.Identity{
		Address: d.priv.Public.Address(),
		Key:     key,
		Tls:     d.priv.Public.IsTLS(),
	}
	prep := &drand.PrepareDKGPacket{
		Node:              id,
		Expected:          uint32(n),
		Threshold:         uint32(thr),
		DkgTimeout:        uint64(dkgTimeout.Seconds()),
		SecretProof:       in.GetInfo().GetSecret(),
		PreviousGroupHash: oldHash,
	}

	// we wait only a certain amount of time for the prepare phase
	nc, cancel := context.WithTimeout(c, MaxWaitPrepareDKG)
	defer cancel()

	// expect group
	err = d.gateway.ProtocolClient.PrepareDKGGroup(nc, lpeer, prep)
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
	newGroup, err := ProtoToGroup(groupPacket)
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
	if dkgInfo.StartTime < now {
		d.log.Error("setup_reshare", "invaid_start_time", "given", dkgInfo.StartTime, "now", now)
		return nil, errors.New("control: invalid start time")
	}

	index, found := newGroup.Index(d.priv.Public)
	if !found {
		// It is ok to not have our key found in the new group since we may just
		// be a node that is leaving the network, but leaving gracefully, by
		// still participating in the resharing.
		d.log.Info("setup_reshare", "not_found_in_new_group")
	}
	d.state.Lock()
	d.index = index
	d.state.Unlock()

	// run the dkg !
	finalGroup, err := d.runResharing(dkgInfo.StartTime, false, oldGroup, newGroup, in.GetInfo().GetTimeout())
	if err != nil {
		return nil, err
	}
	return groupToProto(finalGroup), nil
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
		fmt.Printf(" clock now: %d vs genesis time %d\n\n", d.opts.clock.Now().Unix(), oldGroup.GenesisTime)
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
	var to []*key.Identity
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

	protoGroup := groupToProto(newGroup)
	// we start the dkg at some point in the future - every node will start a
	// this time
	startTime := getDKGStartTime(h.opts.clock, in.GetInfo())
	packet := &drand.PushGroupPacket{
		SecretProof: in.GetInfo().GetSecret(),
		NewGroup:    protoGroup,
		Time:        startTime,
	}
	// send it to everyone in the group nodes
	nodes := group.Nodes
	if err := d.pushDKGInfo(to, packet); err != nil {
		d.log.Error("push_group", err)
		return nil, errors.New("fail to push new group")
	}

	finalGroup, err := d.runResharing(startTime, true, oldGroup, newGroup, in.GetInfo().GetTimeout())
	if err != nil {
		return nil, err
	}
	return groupToProto(finalGroup), nil
}

func (d *Drand) startResharingAsLeader(dkgConf *dkg.Config, oidx int) {
	d.log.With("module", "control").Debug("leader_reshare", "start signalling")
	d.state.Lock()
	msg := &control.ResharePacket{GroupHash: d.nextGroupHash}
	// send resharing packet to signal start of the protocol to other old
	// nodes
	for i, p := range d.nextConf.OldNodes.Identities() {
		if i == oidx {
			continue
		}
		id := p
		ctx, cancel := context.WithTimeout(context.Background(), DefaultDialTimeout)
		defer cancel()
		if _, err := d.gateway.ProtocolClient.ReshareDKG(ctx, id, msg); err != nil {
			//if _, err := d.gateway.InternalClient.Reshare(id, msg, grpc.FailFast(true)); err != nil {
			d.log.With("module", "control").Error("leader_reshare", err)
		}
	}
	d.state.Unlock()
	d.log.With("module", "control").Debug("leader_reshare", "start DKG")
	d.StartDKG(dkgConf)
}

// DistKey returns the distributed key corresponding to the current group
func (d *Drand) DistKey(context.Context, *drand.DistKeyRequest) (*drand.DistKeyResponse, error) {
	pt, err := d.store.LoadDistPublic()
	if err != nil {
		return nil, errors.New("drand: could not load dist. key")
	}
	buff, err := pt.Key().MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &drand.DistKeyResponse{
		Key: buff,
	}, nil
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

// CollectiveKey replies with the distributed key in the response
func (d *Drand) CollectiveKey(ctx context.Context, in *control.CokeyRequest) (*control.CokeyResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()

	key, err := d.store.LoadDistPublic()
	if err != nil {
		return nil, err
	}
	protoKey, err := key.Key().MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &control.CokeyResponse{CoKey: protoKey}, nil
}

// GroupFile replies with the distributed key in the response
func (d *Drand) GroupFile(ctx context.Context, in *control.GroupRequest) (*control.GroupPacket, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.group == nil {
		return nil, errors.New("drand: no dkg group setup yet")
	}
	protoGroup := groupToProto(d.group)
	return protoGroup, nil
}

func (d *Drand) Shutdown(ctx context.Context, in *control.ShutdownRequest) (*control.ShutdownResponse, error) {
	d.Stop()
	return nil, nil
}

func extractGroup(i *control.GroupInfo) (*key.Group, error) {
	var g = &key.Group{}
	switch x := i.Location.(type) {
	case *control.GroupInfo_Path:
		// search group file via local filesystem path
		if err := key.Load(x.Path, g); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("control: can't allow new empty group")
	}
	// run a few checks on the proposed group
	if g.Len() < 4 {
		return nil, errors.New("control: can't accept group with fewer than 4 members")
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

func (d *Drand) getPhaser(timeout uint32) (dkg.Phaser, error) {
	tDuration := time.Duration(timeout) * time.Second
	if timeout == 0 {
		tDuration = DefaultDKGTimeout
	}
	return dkg.NewTimePhaserFunc(func() {
		d.opts.clock.Sleep(tDuration)
		d.log.Debug("phaser", "next_phase")
	}), nil
}

func (d *Drand) pushDKGInfo(to []*key.Identity, packet *drand.DKGInfoPacket) error {
	ctx, cancel := context.WithCancel(context.Background())
	var tooLate = make(chan bool, 1)
	var success = make(chan string, len(to))
	go func() {
		<-d.opts.clock.After(DefaultPushDKGTimeout)
		tooLate <- true
	}()
	for _, node := range to {
		if node.Address() == d.priv.Public.Address() {
			continue
		}
		go func(i *key.Identity) {
			err := d.gateway.ProtocolClient.PushDKGGroup(ctx, i, packet)
			if err != nil {
				d.log.Error("push_dkg", err, "to", i.Address())
			} else {
				success <- i.Address()
			}
		}(node)
	}
	exp := len(to) - 1
	got := 0
	for got < exp {
		select {
		case ok := <-success:
			d.log.Debug("push_dkg", "sending_group", "success_to", ok)
			got++
		case <-tooLate:
			cancel()
			return errors.New("push group timeout")
		}
	}
	d.log.Info("push_dkg", "sending_group", "done")
	return nil
}

func getDKGStartTime(clock clock.Clock, info *control.SetupInfoPacket) int64 {
	offset := info.GetDkgOffset()
	if offset == 0 {
		offset = DefaultDKGOffset
	}

	return clock.Now().Add(time.Duration(offset) * time.Second).Unix()
}

func (d *Drand) waitForDKGStartTime(startTime int64) {
	now := d.opts.clock.Now().Unix()
	if now < startTime {
		d.log.Info("init_dkg", "waiting_start", "sleeping_for", startTime-now)
		d.opts.clock.Sleep(time.Duration(startTime-now) * time.Second)
	}
}

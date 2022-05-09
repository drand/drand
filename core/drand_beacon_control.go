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
	"os"
	"strings"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/beacon"
	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/entropy"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	clock "github.com/jonboulle/clockwork"

	"github.com/drand/drand/net"
	"github.com/drand/kyber/share/dkg"
	vss "github.com/drand/kyber/share/vss/pedersen"

	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
)

// errPreempted is returned on reshares when a subsequent reshare is started concurrently
var errPreempted = errors.New("time out: pre-empted")

// Control services

// InitDKG take a InitDKGPacket, extracts the information needed and wait for
// the DKG protocol to finish. If the request specifies this node is a leader,
// it starts the DKG protocol.
func (bp *BeaconProcess) InitDKG(c context.Context, in *drand.InitDKGPacket) (*drand.GroupPacket, error) {
	bp.state.Lock()
	if bp.dkgDone {
		bp.state.Unlock()
		return nil, errors.New("dkg phase already done - call reshare")
	}
	bp.state.Unlock()

	isLeader := in.GetInfo().GetLeader()

	metrics.DKGStateChange(metrics.DKGWaiting, bp.getBeaconID(), isLeader)

	if !isLeader {
		// different logic for leader than the rest
		out, err := bp.setupAutomaticDKG(c, in)
		return out, err
	}

	bp.log.Infow("", "init_dkg", "begin", "time", bp.opts.clock.Now().Unix(), "leader", true)

	// setup the manager
	newSetup := func(d *BeaconProcess) (*setupManager, error) {
		return newDKGSetup(
			&setupConfig{
				d.log.Named("setupManager"),
				d.opts.clock,
				d.priv.Public,
				in.GetBeaconPeriod(),
				in.GetCatchupPeriod(),
				bp.getBeaconID(),
				in.GetSchemeID(),
				in.GetInfo(),
			},
		)
	}

	// expect the group
	group, err := bp.leaderRunSetup(newSetup)
	if err != nil {
		bp.log.Errorw("", "init_dkg", "leader setup", "err", err)
		return nil, fmt.Errorf("drand: invalid setup configuration: %w", err)
	}

	// send it to everyone in the group nodes
	nodes := group.Nodes
	if err := bp.pushDKGInfo([]*key.Node{}, nodes, 0, group,
		in.GetInfo().GetSecret(), in.GetInfo().GetTimeout()); err != nil {
		return nil, err
	}

	bp.state.Lock()
	// We need to update the leader too
	bp.index = int(group.Find(bp.priv.Public).Index)
	bp.log.Debugw("Starting to use proper node index for logging")
	bp.log = bp.log.Named(fmt.Sprint(bp.index))
	bp.state.Unlock()
	finalGroup, err := bp.runDKG(true, group, in.GetInfo().GetTimeout(), in.GetEntropy())
	if err != nil {
		return nil, err
	}

	response := finalGroup.ToProto(bp.version)

	return response, nil
}

// InitReshare receives information about the old and new group from which to
// operate the resharing protocol.
func (bp *BeaconProcess) InitReshare(c context.Context, in *drand.InitResharePacket) (*drand.GroupPacket, error) {
	oldGroup, err := bp.extractGroup(in.Old)
	if err != nil {
		return nil, err
	}

	oldBeaconID := commonutils.GetCorrectBeaconID(oldGroup.ID)

	if !commonutils.CompareBeaconIDs(oldBeaconID, bp.getBeaconID()) {
		return nil, fmt.Errorf("beacon ID mismatch: "+
			"received group file (%s) ; beaconProcess (%s)", oldBeaconID, bp.getBeaconID())
	}

	isLeader := in.GetInfo().GetLeader()
	metrics.ReshareStateChange(metrics.ReshareWaiting, bp.getBeaconID(), isLeader)
	defer func() {
		metrics.ReshareStateChange(metrics.ReshareIdle, bp.getBeaconID(), false)
	}()

	if !isLeader {
		bp.log.Infow("", "init_reshare", "begin", "leader", false)
		return bp.setupAutomaticResharing(c, oldGroup, in)
	}

	bp.log.Infow("", "init_reshare", "begin", "leader", true, "time", bp.opts.clock.Now())

	newSetup := func(d *BeaconProcess) (*setupManager, error) {
		return newReshareSetup(d.log, d.opts.clock, d.priv.Public, oldGroup, in)
	}

	newGroup, err := bp.leaderRunSetup(newSetup)
	if err != nil {
		bp.log.Errorw("", "init_reshare", "leader setup", "err", err)
		return nil, fmt.Errorf("drand: invalid setup configuration: %w", err)
	}
	if bp.setupCB != nil {
		// XXX Currently a bit hacky - we should split the control plane and the
		// gRPC interface and give that callback as argument
		bp.setupCB(newGroup)
	}
	// some assertions that should always be true but never too safe
	if oldGroup.GenesisTime != newGroup.GenesisTime {
		return nil, errors.New("control: old and new group have different genesis time")
	}
	if oldGroup.GenesisTime > bp.opts.clock.Now().Unix() {
		return nil, errors.New("control: genesis time is in the future")
	}
	if oldGroup.Period != newGroup.Period {
		return nil, errors.New("control: old and new group have different period - unsupported feature at the moment")
	}
	if newGroup.TransitionTime < bp.opts.clock.Now().Unix() {
		return nil, errors.New("control: group with transition time in the past")
	}
	if !bytes.Equal(newGroup.GetGenesisSeed(), oldGroup.GetGenesisSeed()) {
		return nil, errors.New("control: old and new group have different genesis seed")
	}

	// send it to everyone in the group nodes
	if err := bp.pushDKGInfo(oldGroup.Nodes, newGroup.Nodes,
		oldGroup.Threshold,
		newGroup,
		in.GetInfo().GetSecret(),
		in.GetInfo().GetTimeout()); err != nil {
		bp.log.Errorw("", "push_group", err)
		return nil, errors.New("fail to push new group")
	}

	bp.state.Lock()
	oldIdx := bp.index
	// notice that we change the index prior to actually doing the transition
	bp.index = int(newGroup.Find(bp.priv.Public).Index)
	// We need to update the leader too
	bp.log.Debugw("Starting to use new node index for logging", "old", oldIdx, "new", bp.index)
	bp.log = bp.opts.logger.Named(bp.priv.Public.Addr).Named(bp.getBeaconID()).Named(fmt.Sprint(bp.index))
	bp.state.Unlock()

	finalGroup, err := bp.runResharing(true, oldGroup, newGroup, in.GetInfo().GetTimeout())
	if err != nil {
		return nil, err
	}

	response := finalGroup.ToProto(bp.version)

	return response, nil
}

// Share is a functionality of Control Service defined in protobuf/control that requests the private share of the drand node running locally
func (bp *BeaconProcess) Share(ctx context.Context, in *drand.ShareRequest) (*drand.ShareResponse, error) {
	share, err := bp.store.LoadShare()
	if err != nil {
		return nil, err
	}

	id := uint32(share.Share.I)
	buff, err := share.Share.V.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &drand.ShareResponse{Index: id, Share: buff, Metadata: bp.newMetadata()}, nil
}

// PublicKey is a functionality of Control Service defined in protobuf/control
// that requests the long term public key of the drand node running locally
func (bp *BeaconProcess) PublicKey(ctx context.Context, in *drand.PublicKeyRequest) (*drand.PublicKeyResponse, error) {
	bp.state.Lock()
	defer bp.state.Unlock()

	keyPair, err := bp.store.LoadKeyPair()
	if err != nil {
		return nil, err
	}

	protoKey, err := keyPair.Public.Key.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &drand.PublicKeyResponse{PubKey: protoKey, Metadata: bp.newMetadata()}, nil
}

// PrivateKey is a functionality of Control Service defined in protobuf/control
// that requests the long term private key of the drand node running locally
func (bp *BeaconProcess) PrivateKey(ctx context.Context, in *drand.PrivateKeyRequest) (*drand.PrivateKeyResponse, error) {
	bp.state.Lock()
	defer bp.state.Unlock()

	keyPair, err := bp.store.LoadKeyPair()
	if err != nil {
		return nil, err
	}

	protoKey, err := keyPair.Key.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &drand.PrivateKeyResponse{PriKey: protoKey, Metadata: bp.newMetadata()}, nil
}

// GroupFile replies with the distributed key in the response
func (bp *BeaconProcess) GroupFile(ctx context.Context, in *drand.GroupRequest) (*drand.GroupPacket, error) {
	bp.state.Lock()
	defer bp.state.Unlock()

	if bp.group == nil {
		return nil, errors.New("drand: no dkg group setup yet")
	}

	protoGroup := bp.group.ToProto(bp.version)

	return protoGroup, nil
}

// BackupDatabase triggers a backup of the primary database.
func (bp *BeaconProcess) BackupDatabase(ctx context.Context, req *drand.BackupDBRequest) (*drand.BackupDBResponse, error) {
	bp.state.Lock()
	if bp.beacon == nil {
		bp.state.Unlock()
		return nil, errors.New("drand: beacon not setup yet")
	}
	inst := bp.beacon
	bp.state.Unlock()

	w, err := os.OpenFile(req.OutputFile, os.O_WRONLY|os.O_CREATE, os.ModeExclusive)
	if err != nil {
		return nil, fmt.Errorf("could not open file for backup: %w", err)
	}
	defer w.Close()

	return &drand.BackupDBResponse{Metadata: bp.newMetadata()}, inst.Store().SaveTo(w)
}

// ////////

func (bp *BeaconProcess) leaderRunSetup(newSetup func(d *BeaconProcess) (*setupManager, error)) (group *key.Group, err error) {
	// setup the manager
	bp.state.Lock()

	if bp.manager != nil {
		bp.log.Infow("", "reshare", "already_in_progress", "reshare", "restart")
		fmt.Println("\n\n PREEMPTIVE STOP") // nolint
		bp.manager.StopPreemptively()
	}

	manager, err := newSetup(bp)
	bp.log.Infow("", "reshare", "newmanager")
	if err != nil {
		bp.state.Unlock()
		return nil, fmt.Errorf("drand: invalid setup configuration: %w", err)
	}

	go manager.run()

	bp.manager = manager
	bp.state.Unlock()

	defer func() {
		// don't clear manager if pre-empted
		if errors.Is(err, errPreempted) {
			fmt.Println("PREEMPTION ERROR ", err) // nolint
			return
		}
		bp.state.Lock()
		// set back manager to nil afterwards to be able to run a new setup
		bp.manager = nil
		bp.state.Unlock()
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
			bp.log.Debugw("", "init_dkg", "setup_phase", "keys_received", "["+strings.Join(addr, "-")+"]")
		} else {
			bp.log.Debugw("", "init_dkg", "pre-empted")
			return nil, errPreempted
		}
	case <-time.After(MaxWaitPrepareDKG):
		bp.log.Debugw("", "init_dkg", "time_out")
		manager.StopPreemptively()
		return nil, errors.New("time outs: no key received")
	}

	return group, nil
}

// runDKG setups the proper structures and protocol to run the DKG and waits
// until it finishes. If leader is true, this node sends the first packet.
func (bp *BeaconProcess) runDKG(leader bool, group *key.Group, timeout uint32, randomness *drand.EntropyInfo) (*key.Group, error) {
	beaconID := commonutils.GetCorrectBeaconID(group.ID)

	reader, user := extractEntropy(randomness)
	config := &dkg.Config{
		Suite:          key.KeyGroup.(dkg.Suite),
		NewNodes:       group.DKGNodes(),
		Longterm:       bp.priv.Key,
		Reader:         reader,
		UserReaderOnly: user,
		FastSync:       true,
		Threshold:      group.Threshold,
		Nonce:          getNonce(group),
		Auth:           key.DKGAuthScheme,
		Log:            bp.log,
	}
	phaser := bp.getPhaser(timeout)
	board := newEchoBroadcast(bp.log, bp.version, beaconID, bp.privGateway.ProtocolClient,
		bp.priv.Public.Address(), group.Nodes, func(p dkg.Packet) error {
			return dkg.VerifyPacketSignature(config, p)
		})
	dkgProto, err := dkg.NewProtocol(config, board, phaser, true)
	if err != nil {
		return nil, err
	}

	bp.state.Lock()
	dkgInfo := &dkgInfo{
		target: group,
		board:  board,
		phaser: phaser,
		conf:   config,
		proto:  dkgProto,
	}
	bp.dkgInfo = dkgInfo
	if leader {
		bp.dkgInfo.started = true
	}
	metrics.DKGStateChange(metrics.DKGInProgress, beaconID, leader)
	bp.state.Unlock()

	if leader {
		// phaser will kick off the first phase for every other nodes so
		// nodes will send their deals
		bp.log.Infow("", "init_dkg", "START_DKG")
		go phaser.Start()
	}
	bp.log.Infow("", "init_dkg", "wait_dkg_end")
	finalGroup, err := bp.WaitDKG()
	if err != nil {
		bp.log.Errorw("", "init_dkg", err)
		bp.state.Lock()
		if bp.dkgInfo == dkgInfo {
			bp.cleanupDKG()
		}
		bp.state.Unlock()
		return nil, fmt.Errorf("drand: beacon_id [%s] - %w", beaconID, err)
	}
	bp.state.Lock()
	bp.cleanupDKG()
	bp.dkgDone = true
	bp.state.Unlock()

	metrics.DKGStateChange(metrics.DKGDone, beaconID, false)
	bp.log.Infow("", "init_dkg", "dkg_done",
		"starting_beacon_time", finalGroup.GenesisTime, "now", bp.opts.clock.Now().Unix())

	// beacon will start at the genesis time specified
	go bp.StartBeacon(false)

	return finalGroup, nil
}

func (bp *BeaconProcess) cleanupDKG() {
	if bp.dkgInfo != nil {
		bp.dkgInfo.board.Stop()
	}
	bp.dkgInfo = nil
}

// runResharing setups all necessary structures to run the resharing protocol
// and waits until it finishes (or timeouts). If leader is true, it sends the
// first packet so other nodes will start as soon as they receive it.
// nolint:funlen
func (bp *BeaconProcess) runResharing(leader bool, oldGroup, newGroup *key.Group, timeout uint32) (*key.Group, error) {
	oldBeaconID := commonutils.GetCorrectBeaconID(oldGroup.ID)

	oldNode := oldGroup.Find(bp.priv.Public)
	oldPresent := oldNode != nil

	if leader && !oldPresent {
		bp.log.Errorw("", "run_reshare", "invalid", "leader", leader, "old_present", oldPresent)
		return nil, errors.New("can not be a leader if not present in the old group")
	}

	newNode := newGroup.Find(bp.priv.Public)
	newPresent := newNode != nil
	config := &dkg.Config{
		Suite:        key.KeyGroup.(dkg.Suite),
		NewNodes:     newGroup.DKGNodes(),
		OldNodes:     oldGroup.DKGNodes(),
		Longterm:     bp.priv.Key,
		Threshold:    newGroup.Threshold,
		OldThreshold: oldGroup.Threshold,
		FastSync:     true,
		Nonce:        getNonce(newGroup),
		Auth:         key.DKGAuthScheme,
		Log:          bp.log,
	}
	err := func() error {
		bp.state.Lock()
		defer bp.state.Unlock()
		// gives the share to the dkg if we are a current node
		if oldPresent {
			if bp.dkgInfo != nil {
				return errors.New("control: can't reshare from old node when DKG not finished first")
			}
			if bp.share == nil {
				return errors.New("control: can't reshare without a share")
			}
			dkgShare := dkg.DistKeyShare(*bp.share)
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
	var board Broadcast = newEchoBroadcast(bp.log, bp.version, oldBeaconID, bp.privGateway.ProtocolClient,
		bp.priv.Public.Address(), allNodes, func(p dkg.Packet) error {
			return dkg.VerifyPacketSignature(config, p)
		})

	if bp.dkgBoardSetup != nil {
		board = bp.dkgBoardSetup(board)
	}
	phaser := bp.getPhaser(timeout)

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
	bp.state.Lock()
	bp.dkgInfo = info
	if leader {
		bp.log.Infow("", "dkg_reshare", "leader_start",
			"target_group", hex.EncodeToString(newGroup.Hash()), "index", newNode.Index)
		bp.dkgInfo.started = true
	}

	metrics.ReshareStateChange(metrics.ReshareInProgess, oldBeaconID, leader)
	bp.state.Unlock()

	if leader {
		// start the protocol so everyone else follows
		// it sends to all previous and new nodes. old nodes will start their
		// phaser so they will send the deals as soon as they receive this.
		go phaser.Start()
	}

	bp.log.Infow("", "dkg_reshare", "wait_dkg_end")
	finalGroup, err := bp.WaitDKG()
	if err != nil {
		bp.state.Lock()
		if bp.dkgInfo == info {
			bp.cleanupDKG()
		}
		bp.state.Unlock()
		return nil, fmt.Errorf("drand: err during DKG: %w", err)
	}
	bp.log.Infow("", "dkg_reshare", "finished", "leader", leader)
	metrics.ReshareStateChange(metrics.ReshareIdle, oldBeaconID, leader)

	// runs the transition of the beacon
	go bp.transition(oldGroup, oldPresent, newPresent)
	return finalGroup, nil
}

//nolint:funlen
// This method sends the public key to the denoted leader address and then waits
// to receive the group file. After receiving it, it starts the DKG process in
// "waiting" mode, waiting for the leader to send the first packet.
func (bp *BeaconProcess) setupAutomaticDKG(_ context.Context, in *drand.InitDKGPacket) (*drand.GroupPacket, error) {
	bp.log.Infow("", "init_dkg", "begin", "leader", false)

	// determine the leader's address
	laddr := in.GetInfo().GetLeaderAddress()
	lpeer := net.CreatePeer(laddr, in.GetInfo().GetLeaderTls())
	bp.state.Lock()
	if bp.receiver != nil {
		bp.log.Infow("", "dkg_setup", "already_in_progress", "restart", "dkg")
		bp.receiver.stop()
	}
	receiver, err := newSetupReceiver(bp.version, bp.log, bp.opts.clock, bp.privGateway.ProtocolClient, in.GetInfo())
	if err != nil {
		bp.log.Errorw("", "setup", "fail", "err", err)
		bp.state.Unlock()
		return nil, err
	}
	bp.receiver = receiver
	bp.state.Unlock()

	defer func(r *setupReceiver) {
		bp.state.Lock()
		metrics.DKGStateChange(metrics.DKGDone, bp.getBeaconID(), false)
		r.stop()
		if r == bp.receiver {
			// if there has been no new receiver since, we set the field to nil
			bp.receiver = nil
		}
		bp.state.Unlock()
	}(receiver)

	// send public key to leader
	id := bp.priv.Public.ToProto()

	prep := &drand.SignalDKGPacket{
		Node:        id,
		SecretProof: in.GetInfo().GetSecret(),
		Metadata:    bp.newMetadata(),
	}

	bp.log.Debugw("", "init_dkg", "send_key", "leader", lpeer.Address())
	nc, cancel := context.WithTimeout(context.Background(), MaxWaitPrepareDKG)
	defer cancel()

	err = bp.privGateway.ProtocolClient.SignalDKGParticipant(nc, lpeer, prep)
	if err != nil {
		return nil, fmt.Errorf("drand: err when signaling key to leader: %w", err)
	}

	bp.log.Debugw("", "init_dkg", "wait_group")

	group, dkgTimeout, err := bp.receiver.WaitDKGInfo(nc)
	if err != nil {
		return nil, err
	}
	if group == nil {
		bp.log.Debugw("", "init_dkg", "wait_group", "canceled", "nil_group")
		return nil, errors.New("canceled operation")
	}

	now := bp.opts.clock.Now().Unix()
	if group.GenesisTime < now {
		bp.log.Errorw("", "genesis", "invalid", "given", group.GenesisTime)
		return nil, errors.New("control: group with genesis time in the past")
	}

	node := group.Find(bp.priv.Public)
	if node == nil {
		bp.log.Errorw("", "init_dkg", "absent_public_key_in_received_group")
		return nil, errors.New("drand: public key not found in group")
	}
	bp.state.Lock()
	bp.index = int(node.Index)
	bp.log.Debugw("Starting to use proper node index for logging")
	bp.log = bp.log.Named(fmt.Sprint(bp.index))
	bp.state.Unlock()

	// run the dkg
	finalGroup, err := bp.runDKG(false, group, dkgTimeout, in.GetEntropy())
	if err != nil {
		return nil, err
	}
	return finalGroup.ToProto(bp.version), nil
}

// similar to setupAutomaticDKG but with additional verification and information
// w.r.t. to the previous group
// nolint:funlen
func (bp *BeaconProcess) setupAutomaticResharing(_ context.Context, oldGroup *key.Group, in *drand.InitResharePacket) (
	*drand.GroupPacket, error) {
	oldHash := oldGroup.Hash()

	// determine the leader's address
	laddr := in.GetInfo().GetLeaderAddress()
	lpeer := net.CreatePeer(laddr, in.GetInfo().GetLeaderTls())
	bp.state.Lock()
	if bp.receiver != nil {
		if !in.GetInfo().GetForce() {
			bp.log.Infow("", "reshare_setup", "already in progress", "restart", "NOT AUTHORIZED")
			bp.state.Unlock()
			return nil, errors.New("reshare already in progress; use --force")
		}
		bp.log.Infow("", "reshare_setup", "already_in_progress", "restart", "reshare")
		bp.receiver.stop()
		bp.receiver = nil
	}

	receiver, err := newSetupReceiver(bp.version, bp.log, bp.opts.clock, bp.privGateway.ProtocolClient, in.GetInfo())
	if err != nil {
		bp.log.Errorw("", "setup", "fail", "err", err)
		bp.state.Unlock()
		return nil, err
	}
	bp.receiver = receiver
	defer func(r *setupReceiver) {
		metrics.ReshareStateChange(metrics.ReshareIdle, bp.getBeaconID(), false)
		bp.state.Lock()
		r.stop()
		// only set to nil if the given receiver here is the same as the current
		// one, i.e. there has not been a more recent resharing comand issued in
		// between
		if bp.receiver == r {
			bp.receiver = nil
		}
		bp.state.Unlock()
	}(bp.receiver)
	bp.state.Unlock()

	// send public key to leader
	id := bp.priv.Public.ToProto()

	prep := &drand.SignalDKGPacket{
		Node:              id,
		SecretProof:       in.GetInfo().GetSecret(),
		PreviousGroupHash: oldHash,
		Metadata:          bp.newMetadata(),
	}

	metrics.ReshareStateChange(metrics.ReshareWaiting, bp.getBeaconID(), in.GetInfo().GetLeader())

	// we wait only a certain amount of time for the prepare phase
	nc, cancel := context.WithTimeout(context.Background(), MaxWaitPrepareDKG)
	defer cancel()

	bp.log.Infow("", "setup_reshare", "signaling_key_to_leader")
	err = bp.privGateway.ProtocolClient.SignalDKGParticipant(nc, lpeer, prep)
	if err != nil {
		bp.log.Errorw("", "setup_reshare", "failed to signal key to leader", "err", err)
		return nil, fmt.Errorf("drand: err when signaling key to leader: %w", err)
	}

	newGroup, dkgTimeout, err := bp.receiver.WaitDKGInfo(nc)
	if err != nil {
		bp.log.Errorw("", "setup_reshare", "failed to receive dkg info", "err", err)
		return nil, err
	}

	// some assertions that should be true but never too safe
	if err := bp.validateGroupTransition(oldGroup, newGroup); err != nil {
		return nil, err
	}

	node := newGroup.Find(bp.priv.Public)
	if node == nil {
		// It is ok to not have our key found in the new group since we may just
		// be a node that is leaving the network, but leaving gracefully, by
		// still participating in the resharing.
		bp.log.Infow("", "setup_reshare", "not_found_in_new_group")
	} else {
		bp.log.Infow("", "setup_reshare", "participate_newgroup", "new_index", node.Index)
	}

	bp.state.Lock()
	// notice that we are updating the index prior to the actual transition
	oldIdx := bp.index
	bp.index = int(node.Index)
	// we need to change our logger to reflect the potentially changed index
	bp.log.Debugw("Starting to use new node index for logging", "old", oldIdx, "new", bp.index)
	bp.log = bp.opts.logger.Named(bp.priv.Public.Addr).Named(bp.getBeaconID()).Named(fmt.Sprint(bp.index))
	bp.state.Unlock()

	// run the dkg !
	finalGroup, err := bp.runResharing(false, oldGroup, newGroup, dkgTimeout)
	if err != nil {
		bp.log.Errorw("", "setup_reshare", "failed to run resharing", "err", err)
		return nil, err
	}
	return finalGroup.ToProto(bp.version), nil
}

func (bp *BeaconProcess) validateGroupTransition(oldGroup, newGroup *key.Group) error {
	if oldGroup.GenesisTime != newGroup.GenesisTime {
		bp.log.Errorw("", "setup_reshare", "invalid genesis time in received group")
		return errors.New("control: old and new group have different genesis time")
	}

	if oldGroup.Period != newGroup.Period {
		bp.log.Errorw("", "setup_reshare", "invalid period time in received group")
		return errors.New("control: old and new group have different period - unsupported feature at the moment")
	}

	if !commonutils.CompareBeaconIDs(oldGroup.ID, newGroup.ID) {
		bp.log.Errorw("", "setup_reshare", "invalid ID in received group")
		return errors.New("control: old and new group have different ID - unsupported feature at the moment")
	}

	if !bytes.Equal(oldGroup.GetGenesisSeed(), newGroup.GetGenesisSeed()) {
		bp.log.Errorw("", "setup_reshare", "invalid genesis seed in received group")
		return errors.New("control: old and new group have different genesis seed")
	}
	now := bp.opts.clock.Now().Unix()
	if newGroup.TransitionTime < now {
		bp.log.Errorw("", "setup_reshare", "invalid_transition", "given", newGroup.TransitionTime, "now", now)
		return errors.New("control: new group with transition time in the past")
	}
	return nil
}

func (bp *BeaconProcess) extractGroup(old *drand.GroupInfo) (oldGroup *key.Group, err error) {
	bp.state.Lock()
	if oldGroup, err = extractGroup(old); err != nil {
		// try to get the current group
		if bp.group == nil {
			bp.state.Unlock()
			return nil, errors.New("drand: can't init-reshare if no old group provided")
		}
		bp.log.With("module", "control").Debugw("", "init_reshare", "using_stored_group")
		oldGroup = bp.group
		err = nil
	}
	bp.state.Unlock()
	return
}

// PingPong simply responds with an empty packet, proving that this drand node
// is up and alive.
func (bp *BeaconProcess) PingPong(c context.Context, in *drand.Ping) (*drand.Pong, error) {
	return &drand.Pong{Metadata: bp.newMetadata()}, nil
}

func (bp *BeaconProcess) RemoteStatus(c context.Context, in *drand.RemoteStatusRequest) (*drand.RemoteStatusResponse, error) {
	var replies = make(map[string]*drand.StatusResponse)
	for _, addr := range in.GetAddresses() {
		if addr.Address == bp.priv.Public.Addr {
			// no need to reach us
			continue
		}
		p := net.CreatePeer(addr.GetAddress(), addr.Tls)
		resp, err := bp.privGateway.Status(c, p, &drand.StatusRequest{
			CheckConn: in.GetAddresses(),
		})
		if err != nil {
			bp.log.Debug("Remote Status", addr, " FAIL", err)
		} else {
			replies[addr.GetAddress()] = resp
		}
	}
	return &drand.RemoteStatusResponse{
		Statuses: replies,
	}, nil
}

// Status responds with the actual status of drand process
func (bp *BeaconProcess) Status(c context.Context, in *drand.StatusRequest) (*drand.StatusResponse, error) {
	bp.state.Lock()
	defer bp.state.Unlock()

	dkgStatus := drand.DkgStatus{}
	reshareStatus := drand.ReshareStatus{}
	beaconStatus := drand.BeaconStatus{}
	chainStore := drand.ChainStoreStatus{}

	// DKG status
	switch {
	case bp.dkgDone:
		dkgStatus.Status = uint32(DkgReady)
	case !bp.dkgDone && bp.receiver != nil:
		dkgStatus.Status = uint32(DkgInProgress)
	default:
		dkgStatus.Status = uint32(DkgNotStarted)
	}

	// Reshare status
	reshareStatus.Status = uint32(ReshareNotInProgress)
	if bp.dkgDone && bp.receiver != nil {
		reshareStatus.Status = uint32(ReshareInProgress)
	}

	// Beacon status
	beaconStatus.Status = uint32(BeaconNotInited)
	chainStore.IsEmpty = true

	if bp.beacon != nil {
		beaconStatus.Status = uint32(BeaconInited)

		beaconStatus.IsStarted = bp.beacon.IsStarted()
		beaconStatus.IsStopped = bp.beacon.IsStopped()
		beaconStatus.IsRunning = bp.beacon.IsRunning()
		beaconStatus.IsServing = bp.beacon.IsServing()

		// Chain store
		lastBeacon, err := bp.beacon.Store().Last()

		if err == nil && lastBeacon != nil {
			chainStore.IsEmpty = false
			chainStore.LastRound = lastBeacon.GetRound()
			chainStore.Length = uint64(bp.beacon.Store().Len())
		}
	}

	// remote network connectivity
	var resp = make(map[string]bool)
	for _, addr := range in.GetCheckConn() {
		if addr.GetAddress() == bp.priv.Public.Addr {
			continue
		}
		// TODO check if TLS or not
		p := net.CreatePeer(addr.GetAddress(), addr.GetTls())
		// we use an anonymous function to not leak the defer in the for loop
		func() {
			// Simply try to ping him see if he replies
			tc, cancel := context.WithTimeout(c, callMaxTimeout)
			defer cancel()
			_, err := bp.privGateway.Home(tc, p, &drand.HomeRequest{})
			if err != nil {
				bp.log.Debugw("Status asked remote", addr, " FAIL", err)
				resp[addr.GetAddress()] = false
			} else {
				resp[addr.GetAddress()] = true
			}
		}()
	}
	packet := &drand.StatusResponse{
		Dkg:        &dkgStatus,
		Reshare:    &reshareStatus,
		ChainStore: &chainStore,
		Beacon:     &beaconStatus,
	}
	if len(resp) > 0 {
		packet.Connections = resp
	}
	return packet, nil
}

func (bp *BeaconProcess) ListSchemes(c context.Context, in *drand.ListSchemesRequest) (*drand.ListSchemesResponse, error) {
	return &drand.ListSchemesResponse{Ids: scheme.ListSchemes(), Metadata: bp.newMetadata()}, nil
}

func (bp *BeaconProcess) ListBeaconIDs(c context.Context, in *drand.ListSchemesRequest) (*drand.ListSchemesResponse, error) {
	return nil, fmt.Errorf("method not implemented")
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

func (bp *BeaconProcess) getPhaser(timeout uint32) *dkg.TimePhaser {
	tDuration := time.Duration(timeout) * time.Second
	if timeout == 0 {
		tDuration = DefaultDKGTimeout
	}
	// We create a copy of the logger to avoid races when the logger changes
	logger := bp.log
	return dkg.NewTimePhaserFunc(func(phase dkg.Phase) {
		bp.opts.clock.Sleep(tDuration)
		logger.Debugw("", "phaser_finished", phase)
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

// pushDKGInfoPacket sets a specific DKG info packet to specified nodes, and returns a stream of responses.
func (bp *BeaconProcess) pushDKGInfoPacket(ctx context.Context, nodes []*key.Node, packet *drand.DKGInfoPacket) chan pushResult {
	results := make(chan pushResult, len(nodes))

	for _, node := range nodes {
		if node.Address() == bp.priv.Public.Address() {
			continue
		}
		go func(i *key.Identity) {
			err := bp.privGateway.ProtocolClient.PushDKGInfo(ctx, i, packet)
			results <- pushResult{i.Address(), err}
		}(node.Identity)
	}

	return results
}

// pushDKGInfo sends the information to run the DKG to all specified nodes.
// The call is blocking until all nodes have replied or after one minute timeouts.
func (bp *BeaconProcess) pushDKGInfo(outgoing, incoming []*key.Node, previousThreshold int, group *key.Group,
	secret []byte, timeout uint32) error {
	// sign the group to prove you are the leader
	signature, err := key.DKGAuthScheme.Sign(bp.priv.Key, group.Hash())
	if err != nil {
		bp.log.Errorw("", "setup", "leader", "group_signature", err)
		return fmt.Errorf("drand: error signing group: %w", err)
	}

	// Prepare packet
	packet := &drand.DKGInfoPacket{
		NewGroup:    group.ToProto(bp.version),
		SecretProof: secret,
		DkgTimeout:  timeout,
		Signature:   signature,
		Metadata:    bp.newMetadata(),
	}

	// Calculate threshold
	newThreshold := group.Threshold
	if nodesContainAddr(outgoing, bp.priv.Public.Address()) {
		previousThreshold--
	}
	if nodesContainAddr(incoming, bp.priv.Public.Address()) {
		newThreshold--
	}
	to := nodeUnion(outgoing, incoming)

	// Send packet
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultsCh := bp.pushDKGInfoPacket(ctx, to, packet)

	//
	total := len(to) - 1
	for total > 0 {
		select {
		case ok := <-resultsCh:
			total--
			if ok.err != nil {
				bp.log.Errorw("", "push_dkg", "failed to push", "to", ok.address, "err", ok.err)
				continue
			}
			bp.log.Debugw("", "push_dkg", "sending_group", "success_to", ok.address, "left", total)
			if nodesContainAddr(outgoing, ok.address) {
				previousThreshold--
			}
			if nodesContainAddr(incoming, ok.address) {
				newThreshold--
			}
		case <-bp.opts.clock.After(time.Minute):
			if previousThreshold <= 0 && newThreshold <= 0 {
				bp.log.Infow("", "push_dkg", "sending_group", "status", "enough succeeded", "missed", total)
				return nil
			}
			bp.log.Warnw("", "push_dkg", "sending_group", "status", "timeout")
			return errors.New("push group timeout")
		}
	}

	if previousThreshold > 0 || newThreshold > 0 {
		bp.log.Infow("", "push_dkg", "sending_group",
			"status", "not enough succeeded", "prev", previousThreshold, "new", newThreshold)
		return errors.New("push group failure")
	}
	bp.log.Infow("", "push_dkg", "sending_group", "status", "all succeeded")

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
// nolint:funlen
func (bp *BeaconProcess) StartFollowChain(req *drand.StartFollowRequest, stream drand.Control_StartFollowChainServer) error {
	// TODO replace via a more independent chain manager that manages the
	// transition from following -> participating
	bp.state.Lock()

	if bp.syncerCancel != nil {
		bp.state.Unlock()
		return errors.New("syncing is already in progress")
	}

	// context given to the syncer
	// NOTE: this means that if the client quits the requests, the syncing
	// context will signal and it will stop. If we want the following to
	// continue nevertheless we can use the next line by using a new context.
	// ctx, cancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(stream.Context())
	bp.syncerCancel = cancel

	bp.state.Unlock()
	defer func() {
		bp.state.Lock()
		if bp.syncerCancel != nil {
			// it can be nil when we recreate a new beacon we cancel it
			// see drand.go:newBeacon()
			bp.syncerCancel()
		}
		bp.syncerCancel = nil
		bp.state.Unlock()
	}()

	peers := make([]net.Peer, 0, len(req.GetNodes()))
	for _, addr := range req.GetNodes() {
		// we skip our own address
		if addr == bp.priv.Public.Address() {
			continue
		}
		// XXX add TLS disable later
		peers = append(peers, net.CreatePeer(addr, req.GetIsTls()))
	}

	beaconID := bp.getBeaconID()

	info, err := chainInfoFromPeers(stream.Context(), bp.privGateway, peers, bp.log, bp.version, beaconID)
	if err != nil {
		return err
	}

	// we need to get the beaconID from the request since we follow a chain we might not know yet
	hash := req.GetMetadata().GetChainHash()
	if !bytes.Equal(info.Hash(), hash) {
		return fmt.Errorf("chain hash mismatch: rcv(%x) != bp(%x)", info.Hash(), hash)
	}

	bp.log.Debugw("", "start_follow_chain", "fetched chain info", "hash", fmt.Sprintf("%x", info.GenesisSeed))

	if !commonutils.CompareBeaconIDs(beaconID, info.ID) {
		return errors.New("invalid beacon id on chain info")
	}

	store, err := bp.createBoltStore()
	if err != nil {
		bp.log.Errorw("", "start_follow_chain", "unable to create store", "err", err)
		return fmt.Errorf("unable to create store: %w", err)
	}

	// TODO find a better place to put that
	if err := store.Put(chain.GenesisBeacon(info)); err != nil {
		bp.log.Errorw("", "start_follow_chain", "unable to insert genesis block", "err", err)
		store.Close()
		return fmt.Errorf("unable to insert genesis block: %w", err)
	}

	// add scheme store to handle scheme configuration on beacon storing process correctly
	ss := beacon.NewSchemeStore(store, info.Scheme)

	// register callback to notify client of progress
	cbStore := beacon.NewCallbackStore(ss)
	defer cbStore.Close()

	cb, done := sendProgressCallback(stream, req.GetUpTo(), info, bp.opts.clock, bp.log)

	addr := net.RemoteAddress(stream.Context())
	cbStore.AddCallback(addr, cb)
	defer cbStore.RemoveCallback(addr)

	syncer := beacon.NewSyncManager(&beacon.SyncConfig{
		Log:      bp.log,
		Store:    cbStore,
		Info:     info,
		Client:   bp.privGateway,
		Clock:    bp.opts.clock,
		NodeAddr: bp.priv.Public.Address(),
	})
	go syncer.Run()
	defer syncer.Stop()
	syncer.RequestSync(peers, req.GetUpTo())

	// wait for all the callbacks to be called and progress sent before returning
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// chainInfoFromPeers attempts to fetch chain info from one of the passed peers.
func chainInfoFromPeers(ctx context.Context, privGateway *net.PrivateGateway,
	peers []net.Peer, l log.Logger, version commonutils.Version, beaconID string) (*chain.Info, error) {
	request := new(drand.ChainInfoRequest)
	request.Metadata = &common.Metadata{BeaconID: beaconID, NodeVersion: version.ToProto()}

	var info *chain.Info
	for _, peer := range peers {
		ci, err := privGateway.ChainInfo(ctx, peer, request)
		if err != nil {
			l.Debugw("", "start_follow_chain", "error getting chain info", "from", peer.Address(), "err", err)
			continue
		}
		info, err = chain.InfoFromProto(ci)
		if err != nil {
			l.Debugw("", "start_follow_chain", "invalid chain info", "from", peer.Address(), "err", err)
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
			l.Errorw("", "send_progress_callback", "sending_progress", "err", err)
		}
		if upTo > 0 && b.Round == upTo {
			close(done)
		}
	}
	return
}

func extractEntropy(i *drand.EntropyInfo) (io.Reader, bool) {
	if i == nil {
		return nil, false
	}
	r := entropy.NewScriptReader(i.Script)
	user := i.UserOnly
	return r, user
}

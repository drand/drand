package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	clock "github.com/jonboulle/clockwork"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/beacon"
	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/entropy"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share/dkg"
	vss "github.com/drand/kyber/share/vss/pedersen"
)

// errPreempted is returned on reshares when a subsequent reshare is started concurrently
var errPreempted = errors.New("time out: pre-empted")

// Control services

// InitDKG take a InitDKGPacket, extracts the information needed and wait for
// the DKG protocol to finish. If the request specifies this node is a leader,
// it starts the DKG protocol.
func (bp *BeaconProcess) InitDKG(c context.Context, in *drand.InitDKGPacket) (*drand.GroupPacket, error) {
	return nil, nil
}

// InitReshare receives information about the old and new group from which to
// operate the resharing protocol.
//
//nolint:funlen
func (bp *BeaconProcess) InitReshare(c context.Context, in *drand.InitResharePacket) (*drand.GroupPacket, error) {
	return nil, nil
}

// Share is a functionality of Control Service defined in protobuf/control that requests the private share of the drand node running locally
func (bp *BeaconProcess) Share(context.Context, *drand.ShareRequest) (*drand.ShareResponse, error) {
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
func (bp *BeaconProcess) PublicKey(context.Context, *drand.PublicKeyRequest) (*drand.PublicKeyResponse, error) {
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
func (bp *BeaconProcess) PrivateKey(context.Context, *drand.PrivateKeyRequest) (*drand.PrivateKeyResponse, error) {
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
func (bp *BeaconProcess) GroupFile(context.Context, *drand.GroupRequest) (*drand.GroupPacket, error) {
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

	w, err := fs.CreateSecureFile(req.OutputFile)
	if err != nil {
		return nil, fmt.Errorf("could not open file for backup: %w", err)
	}
	defer w.Close()

	return &drand.BackupDBResponse{Metadata: bp.newMetadata()}, inst.Store().SaveTo(ctx, w)
}

// ////////

func (bp *BeaconProcess) leaderRunSetup(newSetup func(d *BeaconProcess) (*setupManager, error)) (group *key.Group, err error) {
	// setup the manager
	bp.state.Lock()

	manager, err := newSetup(bp)
	bp.log.Infow("", "reshare", "newmanager")
	if err != nil {
		bp.state.Unlock()
		return nil, fmt.Errorf("drand: invalid setup configuration: %w", err)
	}

	go manager.run()

	bp.state.Unlock()

	// wait to receive the keys & send them to the other nodes
	var ok bool
	select {
	case group, ok = <-manager.WaitGroup():
		if ok {
			var addr []string
			for _, k := range group.Nodes {
				addr = append(addr, k.Address())
			}
			bp.log.Infow("", "init_dkg", "setup_phase", "keys_received", "["+strings.Join(addr, "-")+"]")
		} else {
			bp.log.Debugw("", "init_dkg", "pre-empted")
			return nil, errPreempted
		}
	case <-time.After(MaxWaitPrepareDKG):
		bp.log.Infow("", "init_dkg", "time_out")
		manager.StopPreemptively()
		return nil, errors.New("time outs: no key received")
	}

	return group, nil
}

// runDKG setups the proper structures and protocol to run the DKG and waits
// until it finishes. If leader is true, this node sends the first packet.
func (bp *BeaconProcess) runDKG(leader bool, group *key.Group, timeout uint32, randomness *drand.EntropyInfo) (*key.Group, error) {
	return nil, nil
}

func (bp *BeaconProcess) cleanupDKG() {

}

// runResharing setups all necessary structures to run the resharing protocol
// and waits until it finishes (or timeouts). If leader is true, it sends the
// first packet so other nodes will start as soon as they receive it.
//
//nolint:funlen
func (bp *BeaconProcess) runResharing(leader bool, oldGroup, newGroup *key.Group, timeout uint32) (*key.Group, error) {
	return nil, nil
}

// This method sends the public key to the denoted leader address and then waits
// to receive the group file. After receiving it, it starts the DKG process in
// "waiting" mode, waiting for the leader to send the first packet.
//
//nolint:funlen
func (bp *BeaconProcess) setupAutomaticDKG(_ context.Context, in *drand.InitDKGPacket) (*drand.GroupPacket, error) {
	return nil, nil
}

// similar to setupAutomaticDKG but with additional verification and information
// w.r.t. to the previous group
//
//nolint:funlen
func (bp *BeaconProcess) setupAutomaticResharing(_ context.Context, oldGroup *key.Group, in *drand.InitResharePacket) (
	*drand.GroupPacket, error,
) {
	return nil, nil
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

func (bp *BeaconProcess) extractGroup(old *drand.GroupInfo) (*key.Group, error) {
	bp.state.Lock()
	defer bp.state.Unlock()

	if oldGroup, err := extractGroup(old); err == nil {
		return oldGroup, nil
	}

	if bp.group == nil {
		return nil, errors.New("can't init-reshare if no old group provided - try providing a group file")
	}

	bp.log.With("module", "control").Debugw("", "init_reshare", "using_stored_group")
	return bp.group, nil
}

// PingPong simply responds with an empty packet, proving that this drand node
// is up and alive.
func (bp *BeaconProcess) PingPong(context.Context, *drand.Ping) (*drand.Pong, error) {
	return &drand.Pong{Metadata: bp.newMetadata()}, nil
}

func (bp *BeaconProcess) RemoteStatus(ctx context.Context, in *drand.RemoteStatusRequest) (*drand.RemoteStatusResponse, error) {
	replies := make(map[string]*drand.StatusResponse)
	bp.log.Debugw("Starting remote status request", "for_nodes", in.GetAddresses())
	for _, addr := range in.GetAddresses() {
		remoteAddress := addr.GetAddress()
		if remoteAddress == "" {
			bp.log.Errorw("Received empty address during remote status", "addr", addr)
			continue
		}

		var err error
		var resp *drand.StatusResponse
		statusReq := &drand.StatusRequest{
			CheckConn: in.GetAddresses(),
			Metadata:  bp.newMetadata(),
		}
		if remoteAddress == bp.priv.Public.Addr {
			// it's ourself
			resp, err = bp.Status(ctx, statusReq)
		} else {
			bp.log.Debugw("Sending status request", "for_node", remoteAddress, "has_TLS", addr.Tls)
			p := net.CreatePeer(remoteAddress, addr.Tls)
			resp, err = bp.privGateway.Status(ctx, p, statusReq)
		}
		if err != nil {
			bp.log.Errorw("Status request failed", "remote", addr, "error", err)
		} else {
			replies[remoteAddress] = resp
		}
	}

	bp.log.Debugw("Done with remote status request", "replies_length", len(replies))
	return &drand.RemoteStatusResponse{
		Statuses: replies,
	}, nil
}

// Status responds with the actual status of drand process
//
//nolint:funlen,gocyclo
func (bp *BeaconProcess) Status(ctx context.Context, in *drand.StatusRequest) (*drand.StatusResponse, error) {
	bp.state.Lock()
	defer bp.state.Unlock()

	bp.log.Debugw("Processing incoming Status request")

	dkgStatus := drand.DkgStatus{}
	//reshareStatus := drand.ReshareStatus{}
	beaconStatus := drand.BeaconStatus{}
	chainStore := drand.ChainStoreStatus{}

	// DKG status
	//dkgStatus.Status = uint32(bp.dkgProcess.Status())
	// prob not accurate, but fix later
	//reshareStatus.Status = uint32(bp.dkgProcess.Status())

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
		lastBeacon, err := bp.beacon.Store().Last(ctx)

		if err == nil && lastBeacon != nil {
			chainStore.IsEmpty = false
			chainStore.LastRound = lastBeacon.GetRound()
			chainStore.Length = lastBeacon.GetRound() + 1
		}
	}

	// remote network connectivity
	nodeList := in.GetCheckConn()
	// in case of an empty list, we test all nodes in the group file
	if len(nodeList) == 0 && bp.beacon != nil && bp.group != nil {
		bp.log.Debugw("Empty node connectivity list, populating with group file")
		for _, node := range bp.group.Nodes {
			nodeList = append(nodeList, &drand.Address{Address: node.Address(), Tls: node.TLS})
		}
	}

	bp.log.Debugw("Starting remote network connectivity check", "for_nodes", nodeList)
	resp := make(map[string]bool)
	for _, addr := range nodeList {
		remoteAddress := addr.GetAddress()
		if remoteAddress == "" {
			bp.log.Warnw("Skipping empty address", "addr", addr)
			continue
		}
		if remoteAddress == bp.priv.Public.Addr {
			// skipping ourself for the connectivity test
			continue
		}

		p := net.CreatePeer(remoteAddress, addr.GetTls())
		// we use an anonymous function to not leak the defer in the for loop
		func() {
			// Simply try to ping him see if he replies
			tc, cancel := context.WithTimeout(ctx, callMaxTimeout)
			defer cancel()
			bp.log.Debugw("Sending Home request", "for_node", remoteAddress, "has_TLS", addr.Tls)
			_, err := bp.privGateway.Home(tc, p, &drand.HomeRequest{Metadata: bp.newMetadata()})
			if err != nil {
				bp.log.Debugw("Status request failed", "remote", addr, "error", err)
				resp[remoteAddress] = false
			} else {
				resp[remoteAddress] = true
			}
		}()
	}
	bp.log.Debugw("Done with connectivity check", "response_length", len(resp))

	packet := &drand.StatusResponse{
		Dkg:        &dkgStatus,
		ChainStore: &chainStore,
		Beacon:     &beaconStatus,
	}
	if len(resp) > 0 {
		packet.Connections = resp
	}
	return packet, nil
}

func (bp *BeaconProcess) ListSchemes(context.Context, *drand.ListSchemesRequest) (*drand.ListSchemesResponse, error) {
	return &drand.ListSchemesResponse{Ids: scheme.ListSchemes(), Metadata: bp.newMetadata()}, nil
}

func (bp *BeaconProcess) ListBeaconIDs(context.Context, *drand.ListSchemesRequest) (*drand.ListSchemesResponse, error) {
	return nil, fmt.Errorf("method not implemented")
}

func extractGroup(i *drand.GroupInfo) (*key.Group, error) {
	g := new(key.Group)
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
		logger.Debugw("phaser timeout", "phaser_finished", phase)
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
		id := node.Identity
		if node.Address() == bp.priv.Public.Address() {
			continue
		}
		go func(i *key.Identity) {
			err := bp.privGateway.ProtocolClient.PushDKGInfo(ctx, i, packet)
			results <- pushResult{i.Address(), err}
		}(id)
	}

	return results
}

// pushDKGInfo sends the information to run the DKG to all specified nodes.
// The call is blocking until all nodes have replied or after one minute timeouts.
func (bp *BeaconProcess) pushDKGInfo(outgoing, incoming []*key.Node, previousThreshold int, group *key.Group,
	secret []byte, timeout uint32,
) error {
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
//
//nolint:funlen
func (bp *BeaconProcess) StartFollowChain(req *drand.StartSyncRequest, stream drand.Control_StartFollowChainServer) error {
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
	if err := store.Put(ctx, chain.GenesisBeacon(info)); err != nil {
		bp.log.Errorw("", "start_follow_chain", "unable to insert genesis block", "err", err)
		store.Close(ctx)
		return fmt.Errorf("unable to insert genesis block: %w", err)
	}

	// add scheme store to handle scheme configuration on beacon storing process correctly
	ss := beacon.NewSchemeStore(store, info.Scheme)

	// register callback to notify client of progress
	cbStore := beacon.NewCallbackStore(ss)
	defer cbStore.Close(ctx)

	cb, done := sendProgressCallback(stream, req.GetUpTo(), info, bp.opts.clock, bp.log)

	addr := net.RemoteAddress(stream.Context())
	cbStore.AddCallback(addr, cb)
	defer cbStore.RemoveCallback(addr)

	syncer := beacon.NewSyncManager(&beacon.SyncConfig{
		Log:         bp.log,
		Store:       cbStore,
		BoltdbStore: store,
		Info:        info,
		Client:      bp.privGateway,
		Clock:       bp.opts.clock,
		NodeAddr:    bp.priv.Public.Address(),
	})
	go syncer.Run()
	defer syncer.Stop()

	for {
		syncer.RequestSync(req.GetUpTo(), peers)
		// wait for all the callbacks to be called and progress sent before returning
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		// we re-send a sync request every 10 periods in case the process got staled and let the sync manager handle
		case <-time.After(info.Period * 10): //nolint:gomnd
			bp.log.Debugw("Sending follow sync request again")
			continue
		}
	}
}

// StartCheckChain checks a chain for validity and pulls invalid beacons from other nodes
func (bp *BeaconProcess) StartCheckChain(req *drand.StartSyncRequest, stream drand.Control_StartCheckChainServer) error {
	logger := bp.log.Named("CheckChain")

	if bp.beacon == nil {
		return errors.New("beacon handler is nil, you might need to first --follow a chain and start aggregating beacons")
	}

	logger.Infow("Starting to check chain for invalid beacons")

	bp.state.Lock()
	if bp.syncerCancel != nil {
		bp.state.Unlock()
		return errors.New("syncing is already in progress")
	}
	// context given to the syncer
	// NOTE: this means that if the client quits the requests, the syncing
	// context will signal it, and it will stop.
	ctx, cancel := context.WithCancel(stream.Context())
	bp.syncerCancel = cancel
	bp.state.Unlock()
	defer func() {
		bp.state.Lock()
		if bp.syncerCancel != nil {
			bp.syncerCancel()
		}
		bp.syncerCancel = nil
		bp.state.Unlock()
	}()

	// we don't monitor the channel for this one, instead we'll error out if needed
	cb, _ := sendPlainProgressCallback(stream, logger, false)

	peers := make([]net.Peer, 0, len(req.GetNodes()))
	for _, addr := range req.GetNodes() {
		// we skip our own address
		if addr == bp.priv.Public.Address() {
			continue
		}
		// XXX add TLS disable later
		peers = append(peers, net.CreatePeer(addr, req.GetIsTls()))
	}

	logger.Debugw("validate_and_sync", "up_to", req.UpTo)
	faultyBeacons, err := bp.beacon.ValidateChain(ctx, req.UpTo, cb)
	if err != nil {
		return err
	}

	// let us reset the progress bar on the client side to track instead the progress of the correction on the beacons
	// this will also pass the "invalid beacon count" to the client through the new target.
	err = stream.Send(&drand.SyncProgress{
		Current: 0,
		Target:  uint64(len(faultyBeacons)),
	})
	if err != nil {
		logger.Errorw("", "send_progress", "sending_progress", "err", err)
	}

	// if we're asking to sync against only us, it's a dry-run
	dryRun := len(req.Nodes) == 1 && req.Nodes[0] == bp.priv.Public.Addr
	if len(faultyBeacons) == 0 || dryRun {
		logger.Infow("Finished without taking any corrective measure", "amount_invalid", len(faultyBeacons), "dry_run", dryRun)
		return nil
	}

	// We need to wait a bit before continuing sending to the client if we don't want mingled output on the client side.
	time.Sleep(time.Second)

	// we need the channel to make sure the client has received the progress
	cb, done := sendPlainProgressCallback(stream, logger, false)

	logger.Infow("Faulty beacons detected in chain, correcting now", "dry-run", false)
	logger.Debugw("Faulty beacons", "List", faultyBeacons)

	err = bp.beacon.CorrectChain(ctx, faultyBeacons, peers, cb)
	if err != nil {
		return err
	}

	// wait for all the callbacks to be called and progress sent before returning
	select {
	case <-done:
		logger.Debugw("Finished correcting chain successfully", "up_to", req.UpTo)
		return nil
	case <-ctx.Done():
		logger.Errorw("Received a cancellation / stream closed", "err", ctx.Err())
		return ctx.Err()
	}
}

// chainInfoFromPeers attempts to fetch chain info from one of the passed peers.
func chainInfoFromPeers(ctx context.Context, privGateway *net.PrivateGateway,
	peers []net.Peer, l log.Logger, version commonutils.Version, beaconID string,
) (*chain.Info, error) {
	// we first craft our request
	request := new(drand.ChainInfoRequest)
	request.Metadata = &common.Metadata{BeaconID: beaconID, NodeVersion: version.ToProto()}

	var info *chain.Info
	var err error
	for _, peer := range peers {
		var ci *drand.ChainInfoPacket
		ci, err = privGateway.ChainInfo(ctx, peer, request)
		if err != nil {
			l.Errorw("", "start_follow_chain", "error getting chain info", "from", peer.Address(), "err", err)
			continue
		}
		info, err = chain.InfoFromProto(ci)
		if err != nil {
			l.Errorw("", "start_follow_chain", "invalid chain info", "from", peer.Address(), "err", err)
			continue
		}
	}
	if info == nil {
		return nil, fmt.Errorf("unable to get chain info successfully. Last err: %w", err)
	}
	return info, nil
}

// sendProgressCallback returns a function that sends SyncProgress on the
// passed stream. It also returns a channel that closes when the callback is
// called with a beacon whose round matches the passed upTo value.
func sendProgressCallback(
	stream drand.Control_StartFollowChainServer, // not ideal since we also reuse it for the StartCheckChain
	upTo uint64,
	info *chain.Info,
	clk clock.Clock,
	l log.Logger,
) (cb func(b *chain.Beacon), done chan struct{}) {
	logger := l.Named("progressCb")
	targ := chain.CurrentRound(clk.Now().Unix(), info.Period, info.GenesisTime)
	if upTo != 0 && upTo < targ {
		targ = upTo
	}

	var plainProgressCb func(a, b uint64)
	plainProgressCb, done = sendPlainProgressCallback(stream, logger, upTo == 0)
	cb = func(b *chain.Beacon) {
		plainProgressCb(b.Round, targ)
	}

	return
}

// sendPlainProgressCallback returns a function that sends SyncProgress on the
// passed stream. It also returns a channel that closes when the callback is
// called with a value whose round matches the passed upTo value.
func sendPlainProgressCallback(
	stream drand.Control_StartFollowChainServer,
	l log.Logger,
	keepFollowing bool,
) (cb func(curr, targ uint64), done chan struct{}) {
	done = make(chan struct{})

	cb = func(curr, targ uint64) {
		// avoids wrapping below and sends latest round number to the client
		if curr > targ {
			targ = curr
		}
		// let us do some rate limiting on the amount of Send we do
		if targ > commonutils.LogsToSkip && targ-curr > commonutils.LogsToSkip && curr%commonutils.LogsToSkip != 0 {
			return
		}

		err := stream.Send(&drand.SyncProgress{
			Current: curr,
			Target:  targ,
		})
		if err != nil {
			l.Errorw("sending_progress", "err", err)
		}
		if !keepFollowing && targ > 0 && curr == targ {
			l.Debugw("target reached", "target", curr)
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

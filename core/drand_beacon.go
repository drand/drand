package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/beacon"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/chain/memdb"
	"github.com/drand/drand/chain/postgresdb/pgdb"
	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/key"
	dlog "github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share/dkg"
)

// BeaconProcess is the main logic of the program. It reads the keys / group file, it
// can start the DKG, read/write shares to files and can initiate/respond to tBLS
// signature requests.
type BeaconProcess struct {
	opts      *Config
	priv      *key.Pair
	beaconID  string
	chainHash []byte
	// current group this drand node is using
	group *key.Group
	index int

	store       key.Store
	dbStore     chain.Store
	privGateway *net.PrivateGateway
	pubGateway  *net.PublicGateway

	beacon *beacon.Handler

	// dkg private share. can be nil if dkg not finished yet.
	share   *key.Share
	dkgDone bool

	// version indicates the base code variant
	version commonutils.Version

	// manager is created and destroyed during a setup phase
	manager  *setupManager
	receiver *setupReceiver

	// dkgInfo contains all the information related to an upcoming or in
	// progress dkg protocol. It is nil for the rest of the time.
	dkgInfo *dkgInfo
	// general logger
	log dlog.Logger

	// global state lock
	state  sync.RWMutex
	exitCh chan bool

	// that cancel function is set when the drand process is following a chain
	// but not participating. Drand calls the cancel func when the node
	// participates to a resharing.
	syncerCancel context.CancelFunc

	// only used for testing currently
	// XXX need boundaries between gRPC and control plane such that we can give
	// a list of parameters at each DKG (including this callback)
	setupCB func(*key.Group)

	// only used for testing at the moment - may be useful later
	// to pinpoint the exact messages from all nodes during dkg
	dkgBoardSetup func(Broadcast) Broadcast
}

func NewBeaconProcess(log dlog.Logger, store key.Store, beaconID string, opts *Config, privGateway *net.PrivateGateway,
	pubGateway *net.PublicGateway) (*BeaconProcess, error) {
	priv, err := store.LoadKeyPair(nil)
	if err != nil {
		return nil, err
	}
	if err := priv.Public.ValidSignature(); err != nil {
		return nil, fmt.Errorf("INVALID SELF SIGNATURE %w. Action: run `drand util self-sign`", err)
	}

	bp := &BeaconProcess{
		beaconID:    commonutils.GetCanonicalBeaconID(beaconID),
		store:       store,
		log:         log,
		priv:        priv,
		version:     commonutils.GetAppVersion(),
		opts:        opts,
		privGateway: privGateway,
		pubGateway:  pubGateway,
		exitCh:      make(chan bool, 1),
	}
	return bp, nil
}

// Load restores a drand instance that is ready to serve randomness, with a
// pre-existing distributed share.
// Returns 'true' if this BeaconProcess is a fresh run, returns 'false' otherwise
func (bp *BeaconProcess) Load() (bool, error) {
	if bp.isFreshRun() {
		return true, nil
	}

	var err error
	bp.group, err = bp.store.LoadGroup()
	if err != nil {
		return false, err
	}

	beaconID := bp.getBeaconID()
	if bp.group == nil {
		bp.dkgDone = false
		metrics.DKGStateChange(metrics.DKGNotStarted, beaconID, false)
		return false, nil
	}

	// this is a migration path to mitigate for the shares being loaded before the group file
	if bp.priv.Public.Scheme.Name == crypto.DefaultSchemeID && crypto.DefaultSchemeID != bp.group.Scheme.Name {
		bp.log.Warnw("Invalid public scheme loaded, reloading key with group's scheme",
			"priv", bp.priv.Public.Scheme.Name, "group", bp.group.Scheme.Name)
		// we need to reload the keypair with the correct scheme
		if bp.priv, err = bp.store.LoadKeyPair(bp.group.Scheme); err != nil {
			return false, err
		}
	}

	bp.state.Lock()
	info := chain.NewChainInfo(bp.group)
	bp.chainHash = info.Hash()
	checkGroup(bp.log, bp.group)
	bp.state.Unlock()

	bp.share, err = bp.store.LoadShare(bp.group.Scheme)
	if err != nil {
		return false, err
	}

	thisBeacon := bp.group.Find(bp.priv.Public)
	if thisBeacon == nil {
		return false, fmt.Errorf("could not restore beacon info for the given identity - this can happen if you updated the group file manually")
	}
	bp.state.Lock()
	bp.index = int(thisBeacon.Index)
	bp.log = bp.log.Named(fmt.Sprint(bp.index))
	bp.state.Unlock()

	bp.log.Debugw("", "serving", bp.priv.Public.Address())
	metrics.DKGStateChange(metrics.DKGDone, beaconID, false)

	return false, nil
}

// WaitDKG waits on the running dkg protocol. In case of an error, it returns
// it. In case of a finished DKG protocol, it saves the dist. public  key and
// private share. These should be loadable by the store.
func (bp *BeaconProcess) WaitDKG() (*key.Group, error) {
	bp.state.RLock()

	if bp.dkgInfo == nil {
		bp.state.RUnlock()
		return nil, errors.New("no dkg info set")
	}

	beaconID := bp.getBeaconID()
	defer func() {
		metrics.DKGStateChange(metrics.DKGDone, beaconID, false)
	}()

	metrics.DKGStateChange(metrics.DKGWaiting, beaconID, false)

	waitCh := bp.dkgInfo.proto.WaitEnd()
	bp.log.Infow("", "waiting_dkg_end", time.Now())

	bp.state.RUnlock()

	res := <-waitCh
	if res.Error != nil {
		return nil, fmt.Errorf("drand: error from dkg: %w", res.Error)
	}

	bp.state.Lock()
	defer bp.state.Unlock()
	// filter the nodes that are not present in the target group
	var qualNodes []*key.Node
	for _, node := range bp.dkgInfo.target.Nodes {
		found := false
		for _, qualNode := range res.Result.QUAL {
			if qualNode.Index == node.Index {
				qualNodes = append(qualNodes, node)
				found = true
				break
			}
		}

		if !found {
			bp.log.Infow("disqualified node during DKG", "node", node)
		}
	}

	s := key.Share{DistKeyShare: *res.Result.Key, Scheme: bp.priv.Scheme()}
	bp.share = &s
	if err := bp.store.SaveShare(bp.share); err != nil {
		return nil, err
	}
	targetGroup := bp.dkgInfo.target
	// only keep the qualified ones
	targetGroup.Nodes = qualNodes
	// setup the dist. public key
	targetGroup.PublicKey = bp.share.Public()
	bp.group = targetGroup
	info := chain.NewChainInfo(targetGroup)
	bp.chainHash = info.Hash()
	output := make([]string, 0, len(qualNodes))
	for _, node := range qualNodes {
		output = append(output, fmt.Sprintf("{addr: %s, idx: %bp, pub: %s}", node.Address(), node.Index, node.Key))
	}
	bp.log.Infow("", "dkg_end", time.Now(), "certified", bp.group.Len(), "list", "["+strings.Join(output, ",")+"]")
	if err := bp.store.SaveGroup(bp.group); err != nil {
		return nil, err
	}
	bp.opts.applyDkgCallback(bp.share, bp.group)
	bp.dkgInfo.board.Stop()
	bp.dkgInfo = nil
	return bp.group, nil
}

// StartBeacon initializes the beacon if needed and launch a go
// routine that runs the generation loop.
func (bp *BeaconProcess) StartBeacon(catchup bool) error {
	ctx := context.Background()
	b, err := bp.newBeacon(ctx)
	if err != nil {
		bp.log.Errorw("", "init_beacon", err)
		return err
	}

	bp.log.Infow("", "beacon_start", bp.opts.clock.Now(), "catchup", catchup)
	if catchup {
		// This doesn't need to be called async.
		// In the future, we might want to wait and return any errors from it too.
		// TODO: Add error handling for this method and handle it here.
		b.Catchup()
	} else if err := b.Start(); err != nil {
		bp.log.Errorw("", "beacon_start", err)
		return err
	}

	return nil
}

// transition between an "old" group and a new group. This method is called
// *after* a resharing dkg has proceed.
// the new beacon syncs before the new network starts
// and will start once the new network time kicks in. The old beacon will stop
// just before the time of the new network.
// TODO: due to current WaitDKG behavior, the old group is overwritten, so an
// old node that fails during the time the resharing is done and the new network
// comes up have to wait for the new network to comes in - that is to be fixed
func (bp *BeaconProcess) transition(oldGroup *key.Group, oldPresent, newPresent bool) {
	// the node should stop a bit before the new round to avoid starting it at
	// the same time as the new node
	// NOTE: this limits the round time of drand - for now it is not a use
	// case to go that fast
	ctx := context.Background()

	// We stop the node at or after transition to make sure they are broadcasting
	// their last partial before transition.
	timeToStop := bp.group.TransitionTime + int64(bp.group.Period.Seconds()) - 1

	if !newPresent {
		// an old node is leaving the network
		if err := bp.beacon.StopAt(timeToStop); err != nil {
			bp.log.Errorw("", "leaving_group", err)
		} else {
			bp.log.Infow("", "leaving_group", "done", "time", bp.opts.clock.Now())
		}
		return
	}

	bp.state.RLock()
	newGroup := bp.group
	newShare := bp.share
	bp.state.RUnlock()

	// tell the current beacon to stop just before the new network starts
	if oldPresent {
		bp.beacon.TransitionNewGroup(newShare, newGroup)
	} else {
		b, err := bp.newBeacon(ctx)
		if err != nil {
			bp.log.Fatalw("", "transition", "new_node", "err", err)
		}
		if err := b.Transition(oldGroup); err != nil {
			bp.log.Errorw("", "sync_before", err)
		}
		bp.log.Infow("", "transition_new", "done")
	}
}

// Stop simply stops all drand operations.
func (bp *BeaconProcess) Stop(ctx context.Context) {
	bp.state.RLock()
	select {
	case <-bp.exitCh:
		bp.log.Errorw("Trying to stop an already stopping beacon process", "id", bp.getBeaconID())
		bp.state.RUnlock()
		return
	default:
		bp.log.Debugw("Stopping BeaconProcess", "id", bp.getBeaconID())
	}

	// we wait until we can send on the channel or the context got canceled
	select {
	case bp.exitCh <- true:
		close(bp.exitCh)
	case <-ctx.Done():
		bp.log.Warnw("Context canceled, BeaconProcess exitCh probably blocked")
	}
	bp.state.RUnlock()

	bp.StopBeacon()
}

// WaitExit returns a channel that signals when drand stops its operations
func (bp *BeaconProcess) WaitExit() chan bool {
	return bp.exitCh
}

func (bp *BeaconProcess) createDBStore(ctx context.Context) (chain.Store, error) {
	beaconName := commonutils.GetCanonicalBeaconID(bp.beaconID)
	var dbStore chain.Store
	var err error

	if bp.group != nil &&
		bp.group.Scheme.Name == crypto.DefaultSchemeID {
		ctx = chain.SetPreviousRequiredOnContext(ctx)
	}

	switch bp.opts.dbStorageEngine {
	case chain.BoltDB:
		dbPath := bp.opts.DBFolder(beaconName)
		fs.CreateSecureFolder(dbPath)
		dbStore, err = boltdb.NewBoltStore(ctx, bp.log, dbPath, bp.opts.boltOpts)

	case chain.MemDB:
		dbStore, err = memdb.NewStore(bp.opts.memDBSize), nil

	case chain.PostgreSQL:
		dbStore, err = pgdb.NewStore(ctx, bp.log, bp.opts.pgConn, beaconName)

	default:
		bp.log.Error("unknown database storage engine type", bp.opts.dbStorageEngine)

		dbPath := bp.opts.DBFolder(beaconName)
		fs.CreateSecureFolder(dbPath)

		dbStore, err = boltdb.NewBoltStore(ctx, bp.log, dbPath, bp.opts.boltOpts)
	}

	bp.dbStore = dbStore
	return dbStore, err
}

func (bp *BeaconProcess) newBeacon(ctx context.Context) (*beacon.Handler, error) {
	bp.state.Lock()
	defer bp.state.Unlock()

	pub := bp.priv.Public
	node := bp.group.Find(pub)

	if node == nil {
		return nil, fmt.Errorf("public key %s not found in group", pub)
	}

	store, err := bp.createDBStore(ctx)
	if err != nil {
		return nil, err
	}

	conf := &beacon.Config{
		Public: node,
		Group:  bp.group,
		Share:  bp.share,
		Clock:  bp.opts.clock,
	}

	if bp.opts.dbStorageEngine == chain.MemDB {
		ctx := context.Background()
		err := bp.storeCurrentFromPeerNetwork(ctx, store)
		if err != nil {
			if errors.Is(err, errNoRoundInPeers) {
				bp.log.Warnw("failed to find target beacon in peer network. Reverting to synced startup", "err", err)
			} else if errors.Is(err, context.DeadlineExceeded) {
				bp.log.Warnw("failed to find target beacon in peer network in a reasonable time. Reverting to synced startup", "err", err)
			} else {
				bp.log.Errorw("got error from storing the beacon in db at startup", "err", err)
				return nil, err
			}
		}
	}

	b, err := beacon.NewHandler(bp.privGateway.ProtocolClient, store, conf, bp.log, bp.version)
	if err != nil {
		return nil, err
	}
	bp.beacon = b
	// cancel any sync operations
	if bp.syncerCancel != nil {
		bp.syncerCancel()
		bp.syncerCancel = nil
	}
	return bp.beacon, nil
}

func checkGroup(l dlog.Logger, group *key.Group) {
	unsigned := group.UnsignedIdentities()
	if unsigned == nil {
		return
	}
	info := make([]string, 0, len(unsigned))
	for _, n := range unsigned {
		info = append(info, fmt.Sprintf("{%s - %s}", n.Address(), key.PointToString(n.Key)[0:10]))
	}
	l.Infow("", "UNSIGNED_GROUP", "["+strings.Join(info, ",")+"]", "FIX", "upgrade")
}

// StopBeacon stops the beacon generation process and resets it.
func (bp *BeaconProcess) StopBeacon() {
	bp.state.Lock()
	defer bp.state.Unlock()
	if bp.beacon == nil {
		return
	}

	bp.beacon.Stop()
	bp.beacon = nil
}

func (bp *BeaconProcess) isFreshRun() bool {
	grp, errG := bp.store.LoadGroup()
	_, errS := bp.store.LoadShare(grp.Scheme)

	isFresh := errG != nil || errS != nil

	bp.log.Debugw("Status when loading group or share", "group error", errG, "share error", errS, "will run as fresh run", isFresh)

	return isFresh
}

// getChainHash return the beaconID of that beaconProcess, if set
func (bp *BeaconProcess) getBeaconID() string {
	return bp.beaconID
}

// getChainHash return the HashChain in hex format as a string
func (bp *BeaconProcess) getChainHash() []byte {
	return bp.chainHash
}

func (bp *BeaconProcess) newMetadata() *common.Metadata {
	metadata := common.NewMetadata(bp.version.ToProto())
	metadata.BeaconID = bp.getBeaconID()

	if hash := bp.getChainHash(); len(hash) > 0 {
		metadata.ChainHash = hash
	}

	return metadata
}

var errNoRoundInPeers = errors.New("could not find round")

func (bp *BeaconProcess) storeCurrentFromPeerNetwork(ctx context.Context, store chain.Store) error {
	clkNow := bp.opts.clock.Now().Unix()
	if bp.group == nil {
		return nil
	}

	targetRound := chain.CurrentRound(clkNow, bp.group.Period, bp.group.GenesisTime)
	bp.log.Debugw("computed the current round", "currentRound", targetRound, "period", bp.group.Period, "genesis", bp.group.GenesisTime)

	//nolint:gomnd // We cannot sync the initial round.
	if targetRound < 2 {
		// Assume this is a fresh start
		return nil
	}

	peers := bp.computePeers(bp.group.Nodes)
	targetBeacon, err := bp.loadBeaconFromPeers(ctx, targetRound, peers)
	if errors.Is(err, errNoRoundInPeers) {
		// If we can't find the desired beacon round, let's try with the latest one.
		// This will work only if the target round is at least 2. Otherwise, we'll
		// start the node from scratch.
		if targetRound > 1 {
			bp.log.Debugw("failed to get target, trying to get the latest round from peers")
			targetBeacon, err = bp.loadBeaconFromPeers(ctx, 0, peers)
		}
	}

	if err != nil {
		bp.log.Debugw("retrieved round error", "err", err, "targetRound", targetRound)
		return err
	}

	err = bp.group.Scheme.VerifyBeacon(&targetBeacon, bp.group.PublicKey.Key())
	if err != nil {
		bp.log.Errorw("failed to verify beacon", "err", err)
		return err
	}

	err = store.Put(ctx, &targetBeacon)
	if err != nil {
		bp.log.Errorw("failed to store beacon", "err", err, "round", targetBeacon.Round)
	} else {
		bp.log.Infow("successfully initialized from peers", "round", targetBeacon.Round)
	}
	return err
}

func (bp *BeaconProcess) loadBeaconFromPeers(ctx context.Context, targetRound uint64, peers []net.Peer) (chain.Beacon, error) {
	select {
	case <-ctx.Done():
		return chain.Beacon{}, ctx.Err()
	default:
	}

	type answer struct {
		peer net.Peer
		b    chain.Beacon
		err  error
	}

	answers := make(chan answer, len(peers))

	//nolint:gomnd //We should search for the beacon for at most 10 seconds.
	ctxFind, cancelFind := context.WithTimeout(ctx, 10*time.Second)
	defer cancelFind()

	prr := drand.PublicRandRequest{
		Round:    targetRound,
		Metadata: bp.newMetadata(),
	}

	for _, peer := range peers {
		go func(peer net.Peer) {
			b := chain.Beacon{}
			r, err := bp.privGateway.PublicRand(ctxFind, peer, &prr)
			if err == nil && r != nil {
				b = chain.Beacon{
					PreviousSig: r.PreviousSignature,
					Round:       r.Round,
					Signature:   r.Signature,
				}
			}
			answers <- answer{peer, b, err}
		}(peer)
	}

	for i := 0; i < len(peers); i++ {
		select {
		case ans := <-answers:
			if ans.err != nil {
				bp.log.Errorw("failed to get rand value from peer", "round", targetRound, "err", ans.err, "peer", ans.peer.Address())
				continue
			}

			bp.log.Infow("returning beacon from peer", "round", ans.b.Round, "peer", ans.peer.Address())

			return ans.b, nil
		case <-ctxFind.Done():
			return chain.Beacon{}, ctxFind.Err()
		case <-ctx.Done():
			return chain.Beacon{}, ctx.Err()
		}
	}

	return chain.Beacon{}, fmt.Errorf("%w %d in any peer", errNoRoundInPeers, targetRound)
}

func (bp *BeaconProcess) computePeers(nodes []*key.Node) []net.Peer {
	nodeAddr := bp.priv.Public.Address()
	var peers []net.Peer
	for i := 0; i < len(nodes); i++ {
		if nodes[i].Address() == nodeAddr {
			// we ignore our own node
			continue
		}

		peers = append(peers, nodes[i].Identity)
	}
	return peers
}

// dkgInfo is a simpler wrapper that keeps the relevant config and logic
// necessary during the DKG protocol.
type dkgInfo struct {
	target  *key.Group
	board   Broadcast
	phaser  *dkg.TimePhaser
	conf    *dkg.Config
	proto   *dkg.Protocol
	started bool
}

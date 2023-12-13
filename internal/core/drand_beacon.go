package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/common"
	public "github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/key"
	dlog "github.com/drand/drand/common/log"
	"github.com/drand/drand/common/tracer"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/internal/chain/beacon"
	"github.com/drand/drand/internal/chain/boltdb"
	"github.com/drand/drand/internal/chain/memdb"
	"github.com/drand/drand/internal/chain/postgresdb/pgdb"
	"github.com/drand/drand/internal/dkg"
	"github.com/drand/drand/internal/fs"
	"github.com/drand/drand/internal/metrics"
	"github.com/drand/drand/internal/net"
	"github.com/drand/drand/internal/util"
	"github.com/drand/drand/protobuf/drand"
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

	beacon        *beacon.Handler
	completedDKGs <-chan dkg.SharingOutput

	// dkg private share. can be nil if dkg not finished yet.
	share *key.Share

	// version indicates the base code variant
	version common.Version

	// general logger
	log dlog.Logger

	// global state lock
	state  sync.RWMutex
	exitCh chan bool

	// that cancel function is set when the drand process is following a chain
	// but not participating. Drand calls the cancel func when the node
	// participates to a resharing.
	syncerCancel context.CancelFunc
}

func NewBeaconProcess(ctx context.Context,
	log dlog.Logger,
	store key.Store,
	completedDKGs chan dkg.SharingOutput,
	beaconID string,
	opts *Config,
	privGateway *net.PrivateGateway) (*BeaconProcess, error) {
	_, span := tracer.NewSpan(ctx, "dd.NewBeaconProcess")
	defer span.End()

	priv, err := store.LoadKeyPair()
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	if err := priv.Public.ValidSignature(); err != nil {
		err := fmt.Errorf("INVALID SELF SIGNATURE %w. Action: run `drand util self-sign`", err)
		span.RecordError(err)
		return nil, err
	}

	bp := &BeaconProcess{
		beaconID:      common.GetCanonicalBeaconID(beaconID),
		store:         store,
		log:           log,
		priv:          priv,
		version:       common.GetAppVersion(),
		opts:          opts,
		privGateway:   privGateway,
		completedDKGs: completedDKGs,
		exitCh:        make(chan bool, 1),
	}
	return bp, nil
}

var ErrDKGNotStarted = errors.New("DKG not started")

// Load restores a drand instance that is ready to serve randomness, with a
// pre-existing distributed share.
func (bp *BeaconProcess) Load(ctx context.Context) error {
	_, span := tracer.NewSpan(ctx, "bp.Load")
	defer span.End()

	var err error

	beaconID := bp.getBeaconID()
	bp.group, err = bp.store.LoadGroup()
	if err != nil || bp.group == nil {
		metrics.DKGStateChange(metrics.DKGNotStarted, beaconID, false)
		span.RecordError(err)
		return ErrDKGNotStarted
	}

	// this is a migration path to mitigate for the shares being loaded before the group file
	if bp.priv.Public.Scheme.Name != bp.group.Scheme.Name {
		bp.log.Errorw("Scheme from share and group did not match. Aborting",
			"priv", bp.priv.Public.Scheme.Name, "group", bp.group.Scheme.Name)
		return fmt.Errorf("scheme mismatch for group or share")
	}

	bp.state.Lock()
	info := public.NewChainInfo(bp.group)
	bp.chainHash = info.Hash()
	checkGroup(bp.log, bp.group)
	bp.state.Unlock()

	bp.share, err = bp.store.LoadShare()
	if err != nil {
		span.RecordError(err)
		return err
	}

	thisBeacon := bp.group.Find(bp.priv.Public)
	if thisBeacon == nil {
		err := fmt.Errorf("could not restore beacon info for the given identity - this can happen if you updated the group file manually")
		span.RecordError(err)
		return err
	}
	bp.state.Lock()
	bp.index = int(thisBeacon.Index)
	bp.log = bp.log.Named(fmt.Sprint(bp.index))
	bp.state.Unlock()

	bp.log.Debugw("", "serving", bp.priv.Public.Address())
	metrics.DKGStateChange(metrics.DKGDone, beaconID, false)

	return nil
}

// StartBeacon initializes the beacon if needed and launch a go
// routine that runs the generation loop.
func (bp *BeaconProcess) StartBeacon(ctx context.Context, catchup bool) error {
	ctx, span := tracer.NewSpan(ctx, "bp.StartBeacon")
	defer span.End()

	b, err := bp.newBeacon(ctx)
	if err != nil {
		span.RecordError(err)
		span.End()
		bp.log.Errorw("", "init_beacon", err)
		return err
	}

	bp.log.Infow("", "beacon_start", bp.opts.clock.Now(), "catchup", catchup)
	if catchup {
		// This doesn't need to be called async.
		// In the future, we might want to wait and return any errors from it too.
		// TODO: Add error handling for this method and handle it here.
		b.Catchup(ctx)
	} else if err := b.Start(ctx); err != nil {
		span.RecordError(err)
		bp.log.Errorw("", "beacon_start", err)
		return err
	}

	return nil
}

func (bp *BeaconProcess) StartListeningForDKGUpdates(ctx context.Context) {
	ctx, span := tracer.NewSpanFromContext(context.Background(), ctx, "bp.StartListeningForDKGUpdates")
	defer span.End()
	for dkgOutput := range bp.completedDKGs {
		if err := bp.onDKGCompleted(ctx, &dkgOutput); err != nil {
			bp.log.Errorw("Error performing DKG key transition", "err", err)
		}
	}
}

// onDKGCompleted transitions between an "old" group and a new group. This method is called
// *after* a DKG has completed.
func (bp *BeaconProcess) onDKGCompleted(ctx context.Context, dkgOutput *dkg.SharingOutput) error {
	ctx, span := tracer.NewSpan(ctx, "bp.onDKGCompleted")
	defer span.End()
	if dkgOutput.BeaconID != bp.beaconID {
		bp.log.Infow(fmt.Sprintf("BeaconProcess for beaconID %s ignoring DKG for beaconID %s", bp.beaconID, dkgOutput.BeaconID))
		return nil
	}

	p, err := util.PublicKeyAsParticipant(bp.priv.Public)
	if err != nil {
		return err
	}

	weWereInLastEpoch := false
	if dkgOutput.Old != nil {
		for _, v := range dkgOutput.Old.FinalGroup.Nodes {
			if v.Addr == p.Address {
				weWereInLastEpoch = true
			}
		}
	}
	weAreInNextEpoch := false
	for _, v := range dkgOutput.New.FinalGroup.Nodes {
		if v.Addr == p.Address {
			weAreInNextEpoch = true
		}
	}

	if weWereInLastEpoch {
		if weAreInNextEpoch {
			return bp.transitionToNext(ctx, dkgOutput)
		}
		return bp.leaveNetwork(ctx)
	}
	if weAreInNextEpoch {
		return bp.joinNetwork(ctx, dkgOutput)
	}

	return errors.New("failed to join the network during the DKG but somehow got to transition")
}

// transitionToNext is called by nodes that were in the network and remain in the network after resharing
func (bp *BeaconProcess) transitionToNext(ctx context.Context, dkgOutput *dkg.SharingOutput) error {
	newGroup := dkgOutput.New.FinalGroup
	newShare := dkgOutput.New.KeyShare

	err := bp.validateGroupTransition(bp.group, newGroup)
	if err != nil {
		return err
	}
	err = bp.storeDKGOutput(ctx, newGroup, newShare)
	if err != nil {
		return err
	}

	// somehow the beacon process isn't set here sometimes o.O
	if bp.beacon == nil {
		return fmt.Errorf("cannot transitionToNext on a nil beacon handler")
	}
	bp.beacon.TransitionNewGroup(ctx, newShare, newGroup)

	return err
}

func (bp *BeaconProcess) storeDKGOutput(ctx context.Context, group *key.Group, share *key.Share) error {
	bp.state.Lock()
	defer bp.state.Unlock()
	bp.group = group
	bp.share = share
	bp.chainHash = public.NewChainInfo(bp.group).Hash()

	err := bp.store.SaveGroup(group)
	if err != nil {
		return err
	}

	err = bp.store.SaveShare(share)
	if err != nil {
		return err
	}

	bp.opts.dkgCallback(ctx, share, group)

	return nil
}

func (bp *BeaconProcess) leaveNetwork(ctx context.Context) error {
	timeToStop := bp.group.TransitionTime - 1
	err := bp.beacon.StopAt(ctx, timeToStop)
	if err != nil {
		bp.log.Errorw("", "leaving_group", err)
	} else {
		bp.log.Infow("", "leaving_group", "done", "time", bp.opts.clock.Now())
	}
	err = bp.store.Reset()
	return err
}

// joinNetwork is called only by new nodes joining a network, not the ones remaining in the network
func (bp *BeaconProcess) joinNetwork(ctx context.Context, dkgOutput *dkg.SharingOutput) error {
	newGroup := dkgOutput.New.FinalGroup
	newShare := dkgOutput.New.KeyShare

	// a node could have left at a prior epoch and rejoined, so make sure the network configuration is still valid
	if bp.group != nil {
		err := bp.validateGroupTransition(bp.group, newGroup)
		if err != nil {
			return err
		}
	}

	err := bp.storeDKGOutput(ctx, newGroup, newShare)
	if err != nil {
		return err
	}

	// if no previous DKG then it's an initial DKG
	// else we need to sync and transition at the right time
	return bp.StartBeacon(ctx, dkgOutput.New.Epoch != 1)
}

// Stop simply stops all drand operations.
func (bp *BeaconProcess) Stop(ctx context.Context) {
	ctx, span := tracer.NewSpan(ctx, "bp.Stop")
	defer span.End()

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

	bp.StopBeacon(ctx)
}

// WaitExit returns a channel that signals when drand stops its operations
func (bp *BeaconProcess) WaitExit() chan bool {
	return bp.exitCh
}

func (bp *BeaconProcess) createDBStore(ctx context.Context) (chain.Store, error) {
	ctx, span := tracer.NewSpan(ctx, "bp.createDBStore")
	defer span.End()

	beaconName := common.GetCanonicalBeaconID(bp.beaconID)
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
	ctx, span := tracer.NewSpan(ctx, "bp.newBeacon")
	defer span.End()

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

	b, err := beacon.NewHandler(ctx, bp.privGateway.ProtocolClient, store, conf, bp.log, bp.version)
	if err != nil {
		return nil, err
	}
	bp.log.Infow("setting handler")
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
	l.Warnw("Group contains invalid signatures", "identities", "["+strings.Join(info, ",")+"]", "FIX", "upgrade")
}

// StopBeacon stops the beacon generation process and resets it.
func (bp *BeaconProcess) StopBeacon(ctx context.Context) {
	ctx, span := tracer.NewSpan(ctx, "bp.StopBeacon")
	defer span.End()

	bp.state.Lock()
	defer bp.state.Unlock()
	if bp.beacon == nil {
		return
	}

	bp.beacon.Stop(ctx)
	bp.beacon = nil
}

// getChainHash return the beaconID of that beaconProcess, if set
func (bp *BeaconProcess) getBeaconID() string {
	return bp.beaconID
}

// getChainHash return the HashChain in hex format as a string
func (bp *BeaconProcess) getChainHash() []byte {
	return bp.chainHash
}

func (bp *BeaconProcess) newMetadata() *drand.Metadata {
	metadata := drand.NewMetadata(bp.version.ToProto())
	metadata.BeaconID = bp.getBeaconID()

	if hash := bp.getChainHash(); len(hash) > 0 {
		metadata.ChainHash = hash
	}

	return metadata
}

var errNoRoundInPeers = errors.New("could not find round")

func (bp *BeaconProcess) storeCurrentFromPeerNetwork(ctx context.Context, store chain.Store) error {
	ctx, span := tracer.NewSpan(ctx, "bp.storeCurrentFromPeerNetwork")
	defer span.End()

	clkNow := bp.opts.clock.Now().Unix()
	if bp.group == nil {
		return nil
	}

	targetRound := common.CurrentRound(clkNow, bp.group.Period, bp.group.GenesisTime)
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

	// if no node in the network has created a beacon yet, we will receive the 0th beacon from them,
	// so we just determine it from our group file and add it, rather that relying on them giving us
	// the correct one
	if targetBeacon.Round == 0 {
		bp.log.Warnw("No node in the network has created a beacon yet: storing genesis beacon instead")
		err = store.Put(ctx, chain.GenesisBeacon(bp.group.GenesisSeed))
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

func (bp *BeaconProcess) loadBeaconFromPeers(ctx context.Context, targetRound uint64, peers []net.Peer) (common.Beacon, error) {
	ctx, span := tracer.NewSpan(ctx, "bp.loadBeaconFromPeers")
	defer span.End()

	select {
	case <-ctx.Done():
		return common.Beacon{}, ctx.Err()
	default:
	}

	type answer struct {
		peer net.Peer
		b    common.Beacon
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
			b := common.Beacon{}
			r, err := bp.privGateway.PublicRand(ctxFind, peer, &prr)
			if err == nil && r != nil {
				b = common.Beacon{
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
			return common.Beacon{}, ctxFind.Err()
		case <-ctx.Done():
			return common.Beacon{}, ctx.Err()
		}
	}

	return common.Beacon{}, fmt.Errorf("%w %d in any peer", errNoRoundInPeers, targetRound)
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

package core

import (
	"context"
	"errors"
	"fmt"
	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/beacon"
	"github.com/drand/drand/chain/boltdb"
	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/core/dkg"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/key"
	dlog "github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/util"
	"strings"
	"sync"
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
	privGateway *net.PrivateGateway
	pubGateway  *net.PublicGateway

	beacon        *beacon.Handler
	completedDKGs <-chan dkg.DKGOutput

	// dkg private share. can be nil if dkg not finished yet.
	share *key.Share

	// version indicates the base code variant
	version commonutils.Version

	// general logger
	log dlog.Logger

	// global state lock
	state  sync.Mutex
	exitCh chan bool

	// that cancel function is set when the drand process is ynollowing a chain
	// but not participating. Drand calls the cancel func when the node
	// participates to a resharing.
	syncerCancel context.CancelFunc

	// only used for testing currently
	// XXX need boundaries between gRPC and control plane such that we can give
	// a list of paramteres at each DKG (inluding this callback)
	setupCB func(*key.Group)
}

func NewBeaconProcess(
	log dlog.Logger,
	store key.Store,
	completedDKGs chan dkg.DKGOutput,
	beaconID string,
	opts *Config,
	privGateway *net.PrivateGateway,
	pubGateway *net.PublicGateway,
) (*BeaconProcess, error) {

	priv, err := store.LoadKeyPair()
	if err != nil {
		return nil, err
	}
	if err := priv.Public.ValidSignature(); err != nil {
		return nil, fmt.Errorf("INVALID SELF SIGNATURE %w. Action: run `drand util self-sign`", err)
	}

	bp := &BeaconProcess{
		beaconID:      commonutils.GetCanonicalBeaconID(beaconID),
		store:         store,
		log:           log,
		priv:          priv,
		version:       commonutils.GetAppVersion(),
		opts:          opts,
		privGateway:   privGateway,
		pubGateway:    pubGateway,
		completedDKGs: completedDKGs,
		exitCh:        make(chan bool, 1),
	}
	return bp, nil
}

var DKGNotStarted = errors.New("DKG not started")

// Load restores a drand instance that is ready to serve randomness, with a
// pre-existing distributed share.
// Returns 'true' if this BeaconProcess is a fresh run, returns 'false' otherwise
func (bp *BeaconProcess) Load() error {
	var err error
	bp.group, err = bp.store.LoadGroup()
	if err != nil {
		return err
	}

	beaconID := bp.getBeaconID()
	if bp.group == nil {
		metrics.DKGStateChange(metrics.DKGNotStarted, beaconID, false)
		return DKGNotStarted
	}

	bp.state.Lock()
	info := chain.NewChainInfo(bp.group)
	bp.chainHash = info.Hash()
	checkGroup(bp.log, bp.group)
	bp.state.Unlock()

	bp.share, err = bp.store.LoadShare()
	if err != nil {
		return err
	}

	thisBeacon := bp.group.Find(bp.priv.Public)
	if thisBeacon == nil {
		return fmt.Errorf("could not restore beacon info for the given identity - this can happen if you updated the group file manually")
	}
	bp.index = int(thisBeacon.Index)
	bp.log = bp.log.Named(fmt.Sprint(bp.index))

	bp.log.Debugw("", "serving", bp.priv.Public.Address())
	metrics.DKGStateChange(metrics.DKGDone, beaconID, false)

	return nil
}

// StartBeacon initializes the beacon if needed and launch a go
// routine that runs the generation loop.
func (bp *BeaconProcess) StartBeacon(catchup bool) {
	b, err := bp.newBeacon()
	if err != nil {
		bp.log.Errorw("", "init_beacon", err)
		return
	}

	bp.log.Infow("", "beacon_start", bp.opts.clock.Now(), "catchup", catchup)
	if catchup {
		go b.Catchup()
	} else if err := b.Start(); err != nil {
		bp.log.Errorw("", "beacon_start", err)
	}

	go bp.StartListeningForDKGUpdates()
}

func (bp *BeaconProcess) StartListeningForDKGUpdates() {
	for {
		select {
		case dkgOutput := <-bp.completedDKGs:
			{
				if err := bp.transition(dkgOutput); err != nil {
					bp.log.Errorw("Error in performing DKG key transition", "err", err)
				}
			}
		}
	}
}

// transition between an "old" group and a new group. This method is called
// *after* a DKG has completed.
func (bp *BeaconProcess) transition(dkgOutput dkg.DKGOutput) error {
	if dkgOutput.BeaconID != bp.beaconID {
		bp.log.Infow(fmt.Sprintf("BeaconProcess for beaconID %s ignoring DKG for beaconID %s", bp.beaconID, dkgOutput.BeaconID))
		return nil
	}

	p, err := util.PublicKeyAsParticipant(bp.priv.Public)
	if err != nil {
		return err
	}

	weWereInLastEpoch := dkgOutput.Old != nil && util.Contains(dkgOutput.Old.FinalGroup, p)
	weAreInNextEpoch := util.Contains(dkgOutput.New.FinalGroup, p)

	if weWereInLastEpoch {
		if weAreInNextEpoch {
			return bp.transitionToNext(dkgOutput)
		}
		return bp.leaveNetwork()
	}
	if weAreInNextEpoch {
		return bp.joinNetwork(dkgOutput)
	}

	return errors.New("failed to join the network during the DKG but somehow got to transition")

}

func (bp *BeaconProcess) transitionToNext(dkgOutput dkg.DKGOutput) error {
	newGroup, err := asGroup(&dkgOutput.New)
	if err != nil {
		return err
	}
	newShare := dkgOutput.New.KeyShare

	err = bp.storeDKGOutput(&newGroup, newShare)
	if err != nil {
		return err
	}
	bp.beacon.TransitionNewGroup(newShare, &newGroup)

	// keep the old beacon running until the `TransitionTime`
	oldGroup, err := asGroup(dkgOutput.Old)
	if err != nil {
		return err
	}
	if err := bp.beacon.Transition(&oldGroup); err != nil {
		bp.log.Errorw("", "sync_before", err)
	} else {
		bp.log.Infow("", "transition_new", "done")
	}

	return err
}

func (bp *BeaconProcess) storeDKGOutput(group *key.Group, share *key.Share) error {
	bp.group = group
	bp.share = share
	bp.chainHash = chain.NewChainInfo(bp.group).Hash()

	err := bp.store.SaveGroup(group)
	if err != nil {
		return err
	}

	err = bp.store.SaveShare(share)
	if err != nil {
		return err
	}

	return nil
}

func (bp *BeaconProcess) leaveNetwork() error {
	timeToStop := bp.group.TransitionTime - 1
	err := bp.beacon.StopAt(timeToStop)
	if err != nil {
		bp.log.Errorw("", "leaving_group", err)
	} else {
		bp.log.Infow("", "leaving_group", "done", "time", bp.opts.clock.Now())
	}
	err = bp.store.Reset()
	return err
}

func (bp *BeaconProcess) joinNetwork(dkgOutput dkg.DKGOutput) error {
	newGroup, err := asGroup(&dkgOutput.New)
	if err != nil {
		return err
	}
	newShare := dkgOutput.New.KeyShare

	err = bp.storeDKGOutput(&newGroup, newShare)
	if err != nil {
		return err
	}

	b, err := bp.newBeacon()
	if err != nil {
		bp.log.Fatalw("", "transition", "new_node", "err", err)
		return err
	}

	bp.beacon = b
	bp.beacon.TransitionNewGroup(newShare, &newGroup)

	syncError := b.Start()
	if syncError != nil {
		b.Catchup()
	}

	return nil
}

func asGroup(details *dkg.DKGState) (key.Group, error) {
	scheme, found := scheme.GetSchemeByID(details.SchemeID)
	if !found {
		return key.Group{}, fmt.Errorf("the schemeID for the given group did not exist, scheme: %s", details.SchemeID)
	}

	finalGroupSorted := util.SortedByPublicKey(details.FinalGroup)
	participantToKeyNode := func(index int, participant *drand.Participant) (*key.Node, error) {
		r, err := util.ToKeyNode(index, participant)
		if err != nil {
			return nil, err
		}
		return &r, nil
	}
	nodes, err := util.TryMapEach[*key.Node](finalGroupSorted, participantToKeyNode)
	if err != nil {
		return key.Group{}, err
	}

	group := key.Group{
		ID:             details.BeaconID,
		Threshold:      int(details.Threshold),
		Period:         details.BeaconPeriod,
		Scheme:         scheme,
		CatchupPeriod:  details.CatchupPeriod,
		Nodes:          nodes,
		GenesisTime:    details.GenesisTime.Unix(),
		GenesisSeed:    details.GenesisSeed,
		TransitionTime: details.TransitionTime.Unix(),
		PublicKey:      details.KeyShare.Public(),
	}

	return group, nil
}

// Stop simply stops all drand operations.
func (bp *BeaconProcess) Stop(ctx context.Context) {
	select {
	case <-bp.exitCh:
		bp.log.Errorw("Trying to stop an already stopping beacon process", "id", bp.getBeaconID())
		return
	default:
		bp.log.Debugw("Stopping BeaconProcess", "id", bp.getBeaconID())
	}
	bp.StopBeacon()
	// we wait until we can send on the channel or the context got canceled
	select {
	case bp.exitCh <- true:
		close(bp.exitCh)
	case <-ctx.Done():
		bp.log.Warnw("Context canceled, BeaconProcess exitCh probably blocked")
	}
}

// WaitExit returns a channel that signals when drand stops its operations
func (bp *BeaconProcess) WaitExit() chan bool {
	return bp.exitCh
}

func (bp *BeaconProcess) createBoltStore() (*boltdb.BoltStore, error) {
	dbName := commonutils.GetCanonicalBeaconID(bp.beaconID)

	dbPath := bp.opts.DBFolder(dbName)
	fs.CreateSecureFolder(dbPath)

	return boltdb.NewBoltStore(bp.log, dbPath, bp.opts.boltOpts)
}

func (bp *BeaconProcess) newBeacon() (*beacon.Handler, error) {
	bp.state.Lock()
	defer bp.state.Unlock()

	pub := bp.priv.Public
	node := bp.group.Find(pub)

	if node == nil {
		return nil, fmt.Errorf("public key %s not found in group", pub)
	}
	conf := &beacon.Config{
		Public: node,
		Group:  bp.group,
		Share:  bp.share,
		Clock:  bp.opts.clock,
	}

	store, err := bp.createBoltStore()
	if err != nil {
		return nil, err
	}

	b, err := beacon.NewHandler(bp.privGateway.ProtocolClient, store, conf, bp.log, bp.version)
	if err != nil {
		return nil, err
	}
	bp.beacon = b
	bp.beacon.AddCallback("opts", bp.opts.callbacks)
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
	_, errG := bp.store.LoadGroup()
	_, errS := bp.store.LoadShare()

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

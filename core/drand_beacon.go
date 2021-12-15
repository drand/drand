package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/common"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/fs"

	"github.com/drand/drand/chain/beacon"
	"github.com/drand/drand/key"
	dlog "github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/kyber/share/dkg"
)

// BeaconProcess is the main logic of the program. It reads the keys / group file, it
// can start the DKG, read/write shars to files and can initiate/respond to TBlS
// signature requests.
type BeaconProcess struct {
	opts *Config
	priv *key.Pair
	// current group this drand node is using
	group *key.Group
	index int

	store       key.Store
	privGateway *net.PrivateGateway
	pubGateway  *net.PublicGateway

	beacon *beacon.Handler

	// dkg private share. can be nil if dkg not finished yet.
	share   *key.Share
	dkgDone bool

	// manager is created and destroyed during a setup phase
	manager  *setupManager
	receiver *setupReceiver

	// dkgInfo contains all the information related to an upcoming or in
	// progress dkg protocol. It is nil for the rest of the time.
	dkgInfo *dkgInfo
	// general logger
	log dlog.Logger

	// global state lock
	state  sync.Mutex
	exitCh chan bool

	// that cancel function is set when the drand process is following a chain
	// but not participating. Drand calls the cancel func when the node
	// participates to a resharing.
	syncerCancel context.CancelFunc

	// only used for testing currently
	// XXX need boundaries between gRPC and control plane such that we can give
	// a list of paramteres at each DKG (inluding this callback)
	setupCB func(*key.Group)

	// version indicates the base code variant
	version common.Version

	// only used for testing at the moment - may be useful later
	// to pinpoint the exact messages from all nodes during dkg
	dkgBoardSetup func(Broadcast) Broadcast
}

func NewBeaconProcess(log dlog.Logger, store key.Store, opts *Config, privGateway *net.PrivateGateway,
	pubGateway *net.PublicGateway) (*BeaconProcess, error) {
	priv, err := store.LoadKeyPair()
	if err != nil {
		return nil, err
	}
	if err := priv.Public.ValidSignature(); err != nil {
		return nil, fmt.Errorf("INVALID SELF SIGNATURE %s. Action: run `drand util self-sign`", err)
	}

	bp := &BeaconProcess{
		store:       store,
		log:         log,
		priv:        priv,
		version:     common.GetAppVersion(),
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

	checkGroup(bp.log, bp.group)
	bp.share, err = bp.store.LoadShare()
	if err != nil {
		return false, err
	}

	bp.log.Debugw("", "beacon_id", bp.group.ID, "serving", bp.priv.Public.Address())
	bp.dkgDone = false

	return false, nil
}

// WaitDKG waits on the running dkg protocol. In case of an error, it returns
// it. In case of a finished DKG protocol, it saves the dist. public  key and
// private share. These should be loadable by the store.
func (bp *BeaconProcess) WaitDKG() (*key.Group, error) {
	bp.state.Lock()

	if bp.dkgInfo == nil {
		bp.state.Unlock()
		return nil, errors.New("no dkg info set")
	}
	waitCh := bp.dkgInfo.proto.WaitEnd()
	bp.log.Debugw("", "beacon_id", bp.dkgInfo.target.ID, "waiting_dkg_end", time.Now())

	bp.state.Unlock()

	res := <-waitCh
	if res.Error != nil {
		return nil, fmt.Errorf("drand: error from dkg: %v", res.Error)
	}

	bp.state.Lock()
	defer bp.state.Unlock()
	// filter the nodes that are not present in the target group
	var qualNodes []*key.Node
	for _, node := range bp.dkgInfo.target.Nodes {
		for _, qualNode := range res.Result.QUAL {
			if qualNode.Index == node.Index {
				qualNodes = append(qualNodes, node)
			}
		}
	}

	s := key.Share(*res.Result.Key)
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
	var output []string
	for _, node := range qualNodes {
		output = append(output, fmt.Sprintf("{addr: %s, idx: %bp, pub: %s}", node.Address(), node.Index, node.Key))
	}
	bp.log.Debugw("", "beacon_id", bp.group.ID, "dkg_end", time.Now(), "certified", bp.group.Len(), "list", "["+strings.Join(output, ",")+"]")
	if err := bp.store.SaveGroup(bp.group); err != nil {
		return nil, err
	}
	bp.opts.applyDkgCallback(bp.share)
	bp.dkgInfo.board.Stop()
	bp.dkgInfo = nil
	return bp.group, nil
}

// StartBeacon initializes the beacon if needed and launch a go
// routine that runs the generation loop.
func (bp *BeaconProcess) StartBeacon(catchup bool) {
	beaconID := bp.group.ID
	b, err := bp.newBeacon()
	if err != nil {
		bp.log.Errorw("", "beacon_id", beaconID, "init_beacon", err)
		return
	}

	bp.log.Infow("", "beacon_id", beaconID, "beacon_start", time.Now(), "catchup", catchup)
	if catchup {
		go b.Catchup()
	} else if err := b.Start(); err != nil {
		bp.log.Errorw("", "beacon_id", beaconID, "beacon_start", err)
	}
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

	beaconID := oldGroup.ID
	timeToStop := bp.group.TransitionTime - 1

	if !newPresent {
		// an old node is leaving the network
		if err := bp.beacon.StopAt(timeToStop); err != nil {
			bp.log.Errorw("", "beacon_id", beaconID, "leaving_group", err)
		} else {
			bp.log.Infow("", "beacon_id", beaconID, "leaving_group", "done", "time", bp.opts.clock.Now())
		}
		return
	}

	bp.state.Lock()
	newGroup := bp.group
	newShare := bp.share
	bp.state.Unlock()

	// tell the current beacon to stop just before the new network starts
	if oldPresent {
		bp.beacon.TransitionNewGroup(newShare, newGroup)
	} else {
		b, err := bp.newBeacon()
		if err != nil {
			bp.log.Fatalw("", "beacon_id", beaconID, "transition", "new_node", "err", err)
		}
		if err := b.Transition(oldGroup); err != nil {
			bp.log.Errorw("", "beacon_id", beaconID, "sync_before", err)
		}
		bp.log.Infow("", "beacon_id", beaconID, "transition_new", "done")
	}
}

// Stop simply stops all drand operations.
func (bp *BeaconProcess) Stop(ctx context.Context) {
	bp.StopBeacon()
	bp.exitCh <- true
}

// WaitExit returns a channel that signals when drand stops its operations
func (bp *BeaconProcess) WaitExit() chan bool {
	return bp.exitCh
}

func (bp *BeaconProcess) createBoltStore(dbName string) (chain.Store, error) {
	if dbName == "" {
		dbName = common.DefaultBeaconID
	}

	dbPath := bp.opts.DBFolder(dbName)
	fs.CreateSecureFolder(dbPath)

	return boltdb.NewBoltStore(dbPath, bp.opts.boltOpts)
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

	store, err := bp.createBoltStore(bp.group.ID)
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
	beaconID := group.ID

	unsigned := group.UnsignedIdentities()
	if unsigned == nil {
		return
	}
	var info []string
	for _, n := range unsigned {
		info = append(info, fmt.Sprintf("{%s - %s}", n.Address(), key.PointToString(n.Key)[0:10]))
	}
	l.Infow("", "beacon_id", beaconID, "UNSIGNED_GROUP", "["+strings.Join(info, ",")+"]", "FIX", "upgrade")
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

	return errG != nil || errS != nil
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

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
	"github.com/drand/drand/common"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/http"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/kyber/share/dkg"
)

// Drand is the main logic of the program. It reads the keys / group file, it
// can start the DKG, read/write shars to files and can initiate/respond to TBlS
// signature requests.
type Drand struct {
	opts *Config
	priv *key.Pair
	// current group this drand node is using
	group *key.Group
	index int

	store       key.Store
	privGateway *net.PrivateGateway
	pubGateway  *net.PublicGateway
	control     net.ControlListener

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
	log log.Logger

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

// NewDrand returns a drand struct. It assumes the private key pair
// has been generated and saved already.
func NewDrand(s key.Store, c *Config) (*Drand, error) {
	d, err := initDrand(s, c)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// initDrand inits the drand struct by loading the private key, and by creating the
// gateway with the correct options.
func initDrand(s key.Store, c *Config) (*Drand, error) {
	logger := c.Logger()
	if !c.insecure && (c.certPath == "" || c.keyPath == "") {
		return nil, errors.New("config: need to set WithInsecure if no certificate and private key path given")
	}
	priv, err := s.LoadKeyPair()
	if err != nil {
		return nil, err
	}
	if err := priv.Public.ValidSignature(); err != nil {
		logger.Errorw("", "INVALID SELF SIGNATURE", err, "action", "run `drand util self-sign`")
	}

	// trick to always set the listening address by default based on the
	// identity. If there is an option to set the address, it will override the
	// default set here..
	d := &Drand{
		store:   s,
		priv:    priv,
		opts:    c,
		log:     logger,
		exitCh:  make(chan bool, 1),
		version: common.GetAppVersion(),
	}
	if err := setupDrand(d, c); err != nil {
		return nil, err
	}
	return d, nil
}

func setupDrand(d *Drand, c *Config) error {
	// Set the private API address to the command-line flag, if given.
	// Otherwise, set it to the address associated with stored private key.
	privAddr := c.PrivateListenAddress(d.priv.Public.Address())
	pubAddr := c.PublicListenAddress("")
	// ctx is used to create the gateway below.
	// Gateway constructors (specifically, the generated gateway stubs that require it)
	// do not actually use it, so we are passing a background context to be safe.
	ctx := context.Background()
	var err error
	d.log.Infow("", "network", "init", "insecure", c.insecure)
	if pubAddr != "" {
		handler, err := http.New(ctx, &drandProxy{d}, c.Version(), d.log.With("server", "http"))
		if err != nil {
			return err
		}
		if d.pubGateway, err = net.NewRESTPublicGateway(ctx, pubAddr, c.certPath, c.keyPath, c.certmanager, handler, c.insecure); err != nil {
			return err
		}
	}
	d.privGateway, err = net.NewGRPCPrivateGateway(ctx, privAddr, c.certPath, c.keyPath, c.certmanager, d, c.insecure, d.opts.grpcOpts...)
	if err != nil {
		return err
	}
	p := c.ControlPort()
	d.control = net.NewTCPGrpcControlListener(d, p)
	go d.control.Start()
	d.log.Infow("", "private_listen", privAddr, "control_port", c.ControlPort(), "public_listen", pubAddr, "folder", d.opts.ConfigFolderMB())
	d.privGateway.StartAll()
	if d.pubGateway != nil {
		d.pubGateway.StartAll()
	}
	return nil
}

// LoadDrand restores a drand instance that is ready to serve randomness, with a
// pre-existing distributed share.
func LoadDrand(s key.Store, c *Config) (*Drand, error) {
	d, err := initDrand(s, c)
	if err != nil {
		return nil, err
	}
	d.group, err = s.LoadGroup()
	if err != nil {
		return nil, err
	}
	checkGroup(d.log, d.group)
	d.share, err = s.LoadShare()
	if err != nil {
		return nil, err
	}
	d.log.Debugw("", "beacon_id", d.group.ID, "serving", d.priv.Public.Address())
	d.dkgDone = true
	return d, nil
}

// WaitDKG waits on the running dkg protocol. In case of an error, it returns
// it. In case of a finished DKG protocol, it saves the dist. public  key and
// private share. These should be loadable by the store.
func (d *Drand) WaitDKG() (*key.Group, error) {
	d.state.Lock()

	if d.dkgInfo == nil {
		d.state.Unlock()
		return nil, errors.New("no dkg info set")
	}
	waitCh := d.dkgInfo.proto.WaitEnd()
	d.log.Debugw("", "beacon_id", d.dkgInfo.target.ID, "waiting_dkg_end", time.Now())

	d.state.Unlock()

	res := <-waitCh
	if res.Error != nil {
		return nil, fmt.Errorf("drand: error from dkg: %v", res.Error)
	}

	d.state.Lock()
	defer d.state.Unlock()
	// filter the nodes that are not present in the target group
	var qualNodes []*key.Node
	for _, node := range d.dkgInfo.target.Nodes {
		for _, qualNode := range res.Result.QUAL {
			if qualNode.Index == node.Index {
				qualNodes = append(qualNodes, node)
			}
		}
	}

	s := key.Share(*res.Result.Key)
	d.share = &s
	if err := d.store.SaveShare(d.share); err != nil {
		return nil, err
	}
	targetGroup := d.dkgInfo.target
	// only keep the qualified ones
	targetGroup.Nodes = qualNodes
	// setup the dist. public key
	targetGroup.PublicKey = d.share.Public()
	d.group = targetGroup
	var output []string
	for _, node := range qualNodes {
		output = append(output, fmt.Sprintf("{addr: %s, idx: %d, pub: %s}", node.Address(), node.Index, node.Key))
	}
	d.log.Debugw("", "beacon_id", d.group.ID, "dkg_end", time.Now(), "certified", d.group.Len(), "list", "["+strings.Join(output, ",")+"]")
	if err := d.store.SaveGroup(d.group); err != nil {
		return nil, err
	}
	d.opts.applyDkgCallback(d.share)
	d.dkgInfo.board.Stop()
	d.dkgInfo = nil
	return d.group, nil
}

// StartBeacon initializes the beacon if needed and launch a go
// routine that runs the generation loop.
func (d *Drand) StartBeacon(catchup bool) {
	beaconID := d.group.ID
	b, err := d.newBeacon()
	if err != nil {
		d.log.Errorw("", "beacon_id", beaconID, "init_beacon", err)
		return
	}

	d.log.Infow("", "beacon_id", beaconID, "beacon_start", time.Now(), "catchup", catchup)
	if catchup {
		go b.Catchup()
	} else if err := b.Start(); err != nil {
		d.log.Errorw("", "beacon_id", beaconID, "beacon_start", err)
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
func (d *Drand) transition(oldGroup *key.Group, oldPresent, newPresent bool) {
	// the node should stop a bit before the new round to avoid starting it at
	// the same time as the new node
	// NOTE: this limits the round time of drand - for now it is not a use
	// case to go that fast

	beaconID := oldGroup.ID
	timeToStop := d.group.TransitionTime - 1

	if !newPresent {
		// an old node is leaving the network
		if err := d.beacon.StopAt(timeToStop); err != nil {
			d.log.Errorw("", "beacon_id", beaconID, "leaving_group", err)
		} else {
			d.log.Infow("", "beacon_id", beaconID, "leaving_group", "done", "time", d.opts.clock.Now())
		}
		return
	}

	d.state.Lock()
	newGroup := d.group
	newShare := d.share
	d.state.Unlock()

	// tell the current beacon to stop just before the new network starts
	if oldPresent {
		d.beacon.TransitionNewGroup(newShare, newGroup)
	} else {
		b, err := d.newBeacon()
		if err != nil {
			d.log.Fatalw("", "beacon_id", beaconID, "transition", "new_node", "err", err)
		}
		if err := b.Transition(oldGroup); err != nil {
			d.log.Errorw("", "beacon_id", beaconID, "sync_before", err)
		}
		d.log.Infow("", "beacon_id", beaconID, "transition_new", "done")
	}
}

// StopBeacon stops the beacon generation process and resets it.
func (d *Drand) StopBeacon() {
	d.state.Lock()
	defer d.state.Unlock()
	if d.beacon == nil {
		return
	}
	d.beacon.Stop()
	d.beacon = nil
}

// Stop simply stops all drand operations.
func (d *Drand) Stop(ctx context.Context) {
	d.StopBeacon()
	d.state.Lock()
	if d.pubGateway != nil {
		d.pubGateway.StopAll(ctx)
	}
	d.privGateway.StopAll(ctx)
	d.control.Stop()
	d.state.Unlock()
	d.exitCh <- true
}

// WaitExit returns a channel that signals when drand stops its operations
func (d *Drand) WaitExit() chan bool {
	return d.exitCh
}

func (d *Drand) createBoltStore(dbName string) (chain.Store, error) {
	if dbName == "" {
		dbName = common.DefaultBeaconID
	}

	dbPath := d.opts.DBFolder(dbName)
	fs.CreateSecureFolder(dbPath)

	return boltdb.NewBoltStore(dbPath, d.opts.boltOpts)
}

func (d *Drand) newBeacon() (*beacon.Handler, error) {
	d.state.Lock()
	defer d.state.Unlock()

	pub := d.priv.Public
	node := d.group.Find(pub)

	if node == nil {
		return nil, fmt.Errorf("public key %s not found in group", pub)
	}
	conf := &beacon.Config{
		Public: node,
		Group:  d.group,
		Share:  d.share,
		Clock:  d.opts.clock,
	}

	store, err := d.createBoltStore(d.group.ID)
	if err != nil {
		return nil, err
	}

	b, err := beacon.NewHandler(d.privGateway.ProtocolClient, store, conf, d.log, d.version)
	if err != nil {
		return nil, err
	}
	d.beacon = b
	d.beacon.AddCallback("opts", d.opts.callbacks)
	// cancel any sync operations
	if d.syncerCancel != nil {
		d.syncerCancel()
		d.syncerCancel = nil
	}
	return d.beacon, nil
}

func checkGroup(l log.Logger, group *key.Group) {
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

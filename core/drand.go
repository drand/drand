package core

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/dkg"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	dkg_proto "github.com/drand/drand/protobuf/crypto/dkg"
	"github.com/drand/drand/protobuf/drand"
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

	store   key.Store
	gateway net.Gateway
	control net.ControlListener

	// handle all callbacks when a new beacon is found
	callbacks *callbackManager

	dkg    *dkg.Handler
	beacon *beacon.Handler
	// dkg private share. can be nil if dkg not finished yet.
	share *key.Share
	// dkg public key. Can be nil if dkg not finished yet.
	pub     *key.DistPublic
	dkgDone bool

	// proposed next group hash for a resharing operation
	nextGroupHash     string
	nextGroup         *key.Group
	nextConf          *dkg.Config
	nextOldPresent    bool // true if we are in the old group
	nextFirstReceived bool // false til receive 1st reshare packet

	// general logger
	log log.Logger

	// global state lock
	state  sync.Mutex
	exitCh chan bool
}

// NewDrand returns an drand struct. It assumes the private key pair
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
	if c.insecure == false && (c.certPath == "" || c.keyPath == "") {
		return nil, errors.New("config: need to set WithInsecure if no certificate and private key path given")
	}
	priv, err := s.LoadKeyPair()
	if err != nil {
		return nil, err
	}

	// trick to always set the listening address by default based on the
	// identity. If there is an option to set the address, it will override the
	// default set here..
	d := &Drand{
		store:     s,
		priv:      priv,
		opts:      c,
		log:       logger,
		exitCh:    make(chan bool, 1),
		callbacks: newCallbackManager(),
	}
	// every new beacon will be passed through the opts callbacks
	d.callbacks.AddCallback(callbackID, d.opts.callbacks)

	a := c.ListenAddress(priv.Public.Address())
	if c.insecure {
		d.log.Info("network", "tls-disable")
		d.gateway = net.NewGrpcGatewayInsecure(a, d, d.opts.grpcOpts...)
	} else {
		d.log.Info("network", "tls-enabled")
		d.gateway = net.NewGrpcGatewayFromCertManager(a, c.certPath, c.keyPath, c.certmanager, d, d.opts.grpcOpts...)
	}
	p := c.ControlPort()
	d.control = net.NewTCPGrpcControlListener(d, p)
	go d.control.Start()
	d.log.Info("network_listen", a, "control_port", c.ControlPort())
	d.gateway.StartAll()
	return d, nil
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
	d.share, err = s.LoadShare()
	if err != nil {
		return nil, err
	}
	d.pub, err = s.LoadDistPublic()
	if err != nil {
		return nil, err
	}
	d.log.Debug("serving", d.priv.Public.Address())
	d.dkgDone = true
	return d, nil
}

// StartDKG starts the DKG protocol by sending the first packet of the DKG
// protocol to every other node in the group. It returns nil if the DKG protocol
// finished successfully or an error otherwise.
func (d *Drand) StartDKG(conf *dkg.Config) error {
	if err := d.createDKG(conf); err != nil {
		return err
	}
	d.dkg.Start()
	return nil
}

// WaitDKG waits on the running dkg protocol. In case of an error, it returns
// it. In case of a finished DKG protocol, it saves the dist. public  key and
// private share. These should be loadable by the store.
func (d *Drand) WaitDKG(conf *dkg.Config) error {
	if err := d.createDKG(conf); err != nil {
		return err
	}

	d.state.Lock()
	waitCh := d.dkg.WaitShare()
	errCh := d.dkg.WaitError()
	d.state.Unlock()

	d.log.Debug("dkg_start", time.Now().String())
	select {
	case share := <-waitCh:
		s := key.Share(share)
		d.share = &s
	case err := <-errCh:
		return fmt.Errorf("drand: error from dkg: %v", err)
	}

	d.state.Lock()
	defer d.state.Unlock()

	d.store.SaveShare(d.share)
	d.store.SaveDistPublic(d.share.Public())
	// XXX change that whole schenanigan
	d.group = d.dkg.QualifiedGroup()
	d.group.Period = conf.NewNodes.Period
	d.group.GenesisTime = conf.NewNodes.GenesisTime
	d.group.TransitionTime = conf.NewNodes.TransitionTime
	d.group.GenesisSeed = conf.NewNodes.GetGenesisSeed()

	d.log.Debug("dkg_end", time.Now(), "certified", d.group.Len())
	d.store.SaveGroup(d.group)
	d.opts.applyDkgCallback(d.share)
	d.dkgDone = true
	d.dkg = nil
	d.nextConf = nil
	return nil
}

// createDKG create the new dkg handler according to the nextConf field. If the
// dkg is not nil, it does not do anything.
func (d *Drand) createDKG(conf *dkg.Config) error {
	d.state.Lock()
	defer d.state.Unlock()
	if d.dkg != nil {
		return nil
	}
	var err error
	if d.dkg, err = dkg.NewHandler(d.dkgNetwork(conf), conf, d.log); err != nil {
		return err
	}
	return nil
}

// StartBeacon initializes the beacon if needed and launch a go
// routine that runs the generation loop.
func (d *Drand) StartBeacon(catchup bool) {
	d.state.Lock()
	defer d.state.Unlock()
	var err error
	d.beacon, err = d.newBeacon()
	if err != nil {
		d.log.Error("init_beacon", err)
		return
	}
	d.log.Info("beacon_start", time.Now(), "catchup", catchup)
	if catchup {
		go d.beacon.Catchup()
	} else {
		if err := d.beacon.Start(); err != nil {
			d.log.Error("beacon_start", err)
		}
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
	replaceBeacon := func() *beacon.Handler {
		d.state.Lock()
		defer d.state.Unlock()
		var err error
		d.beacon, err = d.newBeacon()
		if err != nil {
			d.log.Fatal("new_beacon", err)
		}
		var found bool
		d.index, found = d.group.Index(d.priv.Public)
		if !found {
			d.log.Fatal("transition_index", "absent")
		}
		d.beacon.AddCallback(d.callbacks.NewBeacon)
		return d.beacon
	}
	// the node should stop a bit before the new round to avoid starting it at
	// the same time as the new node
	// NOTE: this limits the round time of drand - for now it is not a use
	// case to go that fast
	timeToStop := d.group.TransitionTime - 1
	if !newPresent {
		//fmt.Printf(" OLD NODE STOPping %s\n", d.priv.Public.Address())
		// an old node is leaving the network
		if err := d.beacon.StopAt(timeToStop); err != nil {
			d.log.Error("leaving_group", err)
		} else {
			d.log.Info("leaving_group", "done", "time", d.opts.clock.Now())
		}
		return
	}
	// tell the current beacon to stop just before the new network starts
	if oldPresent {
		// get reference to last beacon and init the new one
		d.state.Lock()
		currentBeacon := d.beacon
		//share := d.share
		d.state.Unlock()
		currentBeacon.StopAt(timeToStop)
		nbeacon := replaceBeacon()
		//lbeacon, _ := nbeacon.Store().Last()
		//fmt.Printf(" TRANSITION OLD NODE done: node %s - %p : current %d : will stop at %d --> pub %s \n", d.priv.Public.Address(), nbeacon, d.opts.clock.Now().Unix(), timeToStop, share.PubPoly().Eval(1).V.String()[14:19])
		nbeacon.Transition(oldGroup.Nodes)
		d.log.Info("transition_old", "done")
	} else {
		// tell the new node that has "nothing" stored to sync in the meantime
		// and then to start at the time of the new network
		newBeacon := replaceBeacon()
		//fmt.Printf(" TRANSITION NEW NODE: node %d: %s - %p calling transition pub %s\n\n", d.index, d.priv.Public.Address(), d.beacon, d.share.PubPoly().Eval(1).V.String()[14:19])
		if err := newBeacon.Transition(oldGroup.Nodes); err != nil {
			d.log.Error("sync_before", err)
		}
		d.log.Info("transition_new", "done")
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
func (d *Drand) Stop() {
	d.StopBeacon()
	d.state.Lock()
	d.gateway.StopAll()
	d.control.Stop()
	d.state.Unlock()
	d.exitCh <- true
}

// WaitExit returns a channel that signals when drand stops its operations
func (d *Drand) WaitExit() chan bool {
	return d.exitCh
}

// isDKGDone returns true if the DKG protocol has already been executed. That
// means that the only packet that this node should receive are TBLS packet.
func (d *Drand) isDKGDone() bool {
	d.state.Lock()
	defer d.state.Unlock()
	return d.dkgDone
}

func (d *Drand) newBeacon() (*beacon.Handler, error) {
	fs.CreateSecureFolder(d.opts.DBFolder())
	store, err := beacon.NewBoltStore(d.opts.dbFolder, d.opts.boltOpts)
	if err != nil {
		return nil, err
	}
	conf := &beacon.Config{
		Group:   d.group,
		Private: d.priv,
		Share:   d.share,
		Scheme:  key.Scheme,
		Clock:   d.opts.clock,
	}
	return beacon.NewHandler(d.gateway.ProtocolClient, store, conf, d.log)
}

func (d *Drand) beaconCallback(b *beacon.Beacon) {
	d.opts.callbacks(b)
}

// little trick to be able to capture when drand is using the DKG methods,
// instead of offloading that to an external struct without any vision of drand
// internals, or implementing a big "Send" method directly on drand.
func (d *Drand) sendDkgPacket(p net.Peer, pack *dkg_proto.Packet) error {
	_, err := d.gateway.ProtocolClient.Setup(p, &drand.SetupPacket{Dkg: pack})
	return err
}

func (d *Drand) sendResharePacket(p net.Peer, pack *dkg_proto.Packet) error {
	// no concurrency to get nextHash since this is only used within a locked drand
	reshare := &drand.ResharePacket{
		Dkg:       pack,
		GroupHash: d.nextGroupHash,
	}
	_, err := d.gateway.ProtocolClient.Reshare(p, reshare)
	return err
}

func (d *Drand) dkgNetwork(conf *dkg.Config) *dkgNetwork {
	// simple test to check if we are in a resharing mode or in a fresh dkg mode
	// that will lead to two different outer protobuf structures
	if conf.OldNodes == nil {
		return &dkgNetwork{d.sendDkgPacket}
	}
	return &dkgNetwork{d.sendResharePacket}
}

type dkgNetwork struct {
	send func(net.Peer, *dkg_proto.Packet) error
}

func (d *dkgNetwork) Send(p net.Peer, pack *dkg_proto.Packet) error {
	return d.send(p, pack)
}

const callbackID = "callbackID"

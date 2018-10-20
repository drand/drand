package core

import (
	"errors"
	"fmt"
	"sync"

	"github.com/dedis/drand/beacon"
	"github.com/dedis/drand/dkg"
	"github.com/dedis/drand/fs"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	dkg_proto "github.com/dedis/drand/protobuf/dkg"
	"github.com/nikkolasg/slog"
)

// Drand is the main logic of the program. It reads the keys / group file, it
// can start the DKG, read/write shars to files and can initiate/respond to TBlS
// signature requests.
type Drand struct {
	opts *Config
	priv *key.Pair
	// current group this drand node is using
	group *key.Group
	// index in the current group
	idx int

	store   key.Store
	gateway net.Gateway

	dkg         *dkg.Handler
	beacon      *beacon.Handler
	beaconStore beacon.Store
	// dkg private share. can be nil if dkg not finished yet.
	share *key.Share
	// dkg public key. Can be nil if dkg not finished yet.
	pub     *key.DistPublic
	dkgDone bool

	// proposed next group hash for a resharing operation
	nextGroupHash string
	nextGroup     *key.Group
	nextConf      *dkg.Config

	// global state lock
	state sync.Mutex
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
		store: s,
		priv:  priv,
		opts:  c,
	}

	a := c.ListenAddress(priv.Public.Address())
	p := c.ControlPort()
	if c.insecure {
		d.gateway = net.NewGrpcGatewayInsecure(a, p, d, d, d.opts.grpcOpts...)
	} else {
		d.gateway = net.NewGrpcGatewayFromCertManager(a, p, c.certPath, c.keyPath, c.certmanager, d, d, d.opts.grpcOpts...)
	}
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
	if err := d.initBeacon(); err != nil {
		return nil, err
	}
	slog.Debugf("drand: loaded and serving at %s", d.priv.Public.Address())
	return d, nil
}

// StartDKG starts the DKG protocol by sending the first packet of the DKG
// protocol to every other node in the group. It returns nil if the DKG protocol
// finished successfully or an error otherwise.
func (d *Drand) StartDKG() error {
	if err := d.createDKG(); err != nil {
		return err
	}
	d.dkg.Start()
	return nil
}

// WaitDKG waits on the running dkg protocol. In case of an error, it returns
// it. In case of a finished DKG protocol, it saves the dist. public  key and
// private share. These should be loadable by the store.
func (d *Drand) WaitDKG() error {
	if err := d.createDKG(); err != nil {
		return err
	}

	d.state.Lock()
	waitCh := d.dkg.WaitShare()
	errCh := d.dkg.WaitError()
	d.state.Unlock()

	var err error
	select {
	case share := <-waitCh:
		s := key.Share(share)
		d.share = &s
	case err = <-errCh:
	}
	if err != nil {
		return err
	}

	d.state.Lock()
	defer d.state.Unlock()

	d.store.SaveShare(d.share)
	d.store.SaveDistPublic(d.share.Public())
	// XXX change to qualified group
	d.group = d.nextConf.NewNodes
	d.group.PublicKey = d.share.Public()
	d.store.SaveGroup(d.group)
	d.dkgDone = true
	d.dkg = nil
	d.nextConf = nil
	d.beacon = nil // to allow re-creation of beacon with new share
	return nil
}

// createDKG create the new dkg handler according to the nextConf field. If the
// dkg is not nil, it does not do anything.
func (d *Drand) createDKG() error {
	d.state.Lock()
	defer d.state.Unlock()
	if d.dkg != nil {
		return nil
	}
	if d.nextConf == nil {
		return errors.New("drand: invalid state -> nil nextConf")
	}
	var err error
	c := d.nextConf
	if d.dkg, err = dkg.NewHandler(d.dkgNetwork(c), c); err != nil {
		return err
	}
	return nil
}

var DefaultSeed = []byte("Truth is like the sun. You can shut it out for a time, but it ain't goin' away.")

func (d *Drand) StartBeacon() error {
	if err := d.initBeacon(); err != nil {
		return err
	}
	go d.BeaconLoop()
	return nil
}

// BeaconLoop starts periodically the TBLS protocol. The seed is the first
// message signed alongside with the current timestamp. All subsequent
// signatures are chained:
// s_i+1 = SIG(s_i || timestamp)
// For the moment, each resulting signature is stored in a file named
// beacons/<timestamp>.sig.
// The period is determined according the group.toml this node belongs to.
func (d *Drand) BeaconLoop() error {
	d.state.Lock()
	// heuristic: we catchup when we can retrieve a beacon from the db
	// if there is an error we quit, if there is no beacon saved yet, we
	// run the loop as usual.
	var catchup = true
	_, err := d.beaconStore.Last()
	if err != nil {
		if err == beacon.ErrNoBeaconSaved {
			// we are starting the beacon generation
			catchup = false
		} else {
			// there's a serious error
			d.state.Unlock()
			return fmt.Errorf("drand: could not determine beacon state: %s", err)
		}
	}
	if catchup {
		slog.Infof("drand: starting beacon loop in catch-up mode")
	} else {
		slog.Infof("drand: starting beacon loop")
	}
	period := getPeriod(d.group)
	d.state.Unlock()

	d.beacon.Loop(DefaultSeed, period, catchup)
	return nil
}

func (d *Drand) Stop() {
	d.state.Lock()
	defer d.state.Unlock()
	d.gateway.StopAll()
	if d.beacon != nil {
		d.beacon.Stop()
	}
}

// isDKGDone returns true if the DKG protocol has already been executed. That
// means that the only packet that this node should receive are TBLS packet.
func (d *Drand) isDKGDone() bool {
	d.state.Lock()
	defer d.state.Unlock()
	return d.dkgDone
}

func (d *Drand) initBeacon() error {
	d.state.Lock()
	defer d.state.Unlock()
	if d.beacon != nil {
		return nil
	}
	d.dkgDone = true
	fs.CreateSecureFolder(d.opts.DBFolder())
	store, err := beacon.NewBoltStore(d.opts.dbFolder, d.opts.boltOpts)
	if err != nil {
		return err
	}
	d.beaconStore = beacon.NewCallbackStore(store, d.beaconCallback)
	d.beacon = beacon.NewHandler(d.gateway.InternalClient, d.priv, d.share, d.group, d.beaconStore)
	return nil
}

func (d *Drand) beaconCallback(b *beacon.Beacon) {
	d.opts.callbacks(b)
}

// little trick to be able to capture when drand is using the DKG methods,
// instead of offloading that to an external struct without any vision of drand
// internals, or implementing a big "Send" method directly on drand.
func (d *Drand) sendDkgPacket(p net.Peer, pack *dkg_proto.DKGPacket) error {
	_, err := d.gateway.InternalClient.Setup(p, pack)
	return err
}

func (d *Drand) sendResharePacket(p net.Peer, pack *dkg_proto.DKGPacket) error {
	// no concurrency to get nextHash since this is only used within a locked drand
	reshare := &dkg_proto.ResharePacket{
		Packet:    pack,
		GroupHash: d.nextGroupHash,
	}
	_, err := d.gateway.InternalClient.Reshare(p, reshare)
	return err
}

func (d *Drand) dkgNetwork(conf *dkg.Config) *dkgNetwork {
	// simple test to check if we are in a resharing mode or in a fresh dkg mode
	// that will lead to two different outer protobuf structure
	if conf.OldNodes == nil {
		return &dkgNetwork{d.sendDkgPacket}
	}
	return &dkgNetwork{d.sendResharePacket}
}

type dkgNetwork struct {
	send func(net.Peer, *dkg_proto.DKGPacket) error
}

func (d *dkgNetwork) Send(p net.Peer, pack *dkg_proto.DKGPacket) error {
	return d.send(p, pack)
}

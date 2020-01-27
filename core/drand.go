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
	// index in the current group
	idx int

	store   key.Store
	gateway net.Gateway
	control net.ControlListener

	dkg         *dkg.Handler
	beacon      *beacon.Handler
	beaconStore beacon.Store
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
		store: s,
		priv:  priv,
		opts:  c,
		log:   logger,
	}

	a := c.ListenAddress(priv.Public.Address())
	if c.insecure {
		d.gateway = net.NewGrpcGatewayInsecure(a, d, d.opts.grpcOpts...)
	} else {
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
	d.log.Debug("drand: loaded and serving at %s", d.priv.Public.Address())
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
	d.group = d.dkg.QualifiedGroup()
	// need to save the period before since dkg returns a *new* fresh group, it
	// does not know about the period.
	d.group.Period = d.nextConf.NewNodes.Period
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
func (d *Drand) createDKG() error {
	d.state.Lock()
	defer d.state.Unlock()
	if d.dkg != nil {
		return nil
	}
	if d.nextConf == nil {
		return errors.New("drand: invalid state: no next configuration")
	}
	var err error
	c := d.nextConf
	if d.dkg, err = dkg.NewHandler(d.dkgNetwork(c), c, d.log); err != nil {
		return err
	}
	return nil
}

// DefaultSeed is the message signed during the first beacon generation,
// alongside with the round number 0.
var DefaultSeed = []byte("Truth is like the sun. You can shut it out for a time, but it ain't goin' away.")

// StartBeacon initializes the beacon if needed and launch a go routine that
// runs the generation loop.
func (d *Drand) StartBeacon(catchup bool) error {
	d.state.Lock()
	defer d.state.Unlock()
	if d.beacon == nil {
		d.state.Unlock()
		d.initBeacon()
		d.state.Lock()
	}
	period := getPeriod(d.group)
	d.log.Info("beacon_start", time.Now(), "catchup", catchup)
	go d.beacon.Run(period, catchup)
	return nil
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
	fs.CreateSecureFolder(d.opts.DBFolder())
	store, err := beacon.NewBoltStore(d.opts.dbFolder, d.opts.boltOpts)
	if err != nil {
		return err
	}
	d.beaconStore = beacon.NewCallbackStore(store, d.beaconCallback)
	conf := &beacon.Config{
		Group:   d.group,
		Private: d.priv,
		Share:   d.share,
		Seed:    DefaultSeed,
		Scheme:  key.Scheme,
		Clock:   d.opts.clock,
	}
	d.beacon, err = beacon.NewHandler(d.gateway.ProtocolClient, d.beaconStore, conf, d.log)
	return err
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

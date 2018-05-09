package core

import (
	"context"
	"errors"
	"sync"

	"github.com/dedis/drand/dkg"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	dkg_proto "github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
)

// Drand is the main logic of the program. It reads the keys / group file, it
// can start the DKG, read/write shars to files and can initiate/respond to TBlS
// signature requests.
type Drand struct {
	opts    *drandOpts
	priv    *key.Private
	group   *key.Group
	store   key.Store
	gateway net.Gateway

	dkg *dkg.Handler
	//beacon *Beacon

	share   *key.Share // dkg private share. can be nil if dkg not executed.
	dkgDone bool

	state sync.Mutex
	done  chan bool
}

// NewDrand returns an drand struct that is ready to start the DKG protocol with
// the given group and then to serve randomness. It assumes the private key pair
// has been generated already.
func NewDrand(s key.Store, g *key.Group, opts ...DrandOptions) (*Drand, error) {
	d, err := initDrand(s, g, opts...)
	if err != nil {
		return nil, err
	}
	dkgConf := &dkg.Config{
		Suite:   key.G2.(dkg.Suite),
		Group:   g,
		Timeout: d.opts.dkgTimeout,
	}
	d.dkg, err = dkg.NewHandler(d.priv, dkgConf, d.dkgNetwork())
	return d, err
}

// initDrand inits the drand struct by loading the private key, and by creating the
// gateway with the correct options.
func initDrand(s key.Store, g *key.Group, opts ...DrandOptions) (*Drand, error) {
	d := &Drand{store: s, opts: newDrandOpts(opts...)}
	var err error
	d.priv, err = s.LoadPrivate()
	if err != nil {
		return nil, err
	}
	d.group = g
	d.gateway = net.NewGrpcGateway(d.priv, d, d.opts.grpcOpts...)
	go d.gateway.Start()
	d.done = make(chan bool, 1)
	return d, nil
}

/*// LoadDrand restores a drand instance as it was running after a DKG instance*/
//func LoadDrand(s key.Store) (*Drand, error) {
//group, err := s.LoadGroup()
//if err != nil {
//return nil, err
//}
//d, err := newDrand(priv, group, s)
//if err != nil {
//return nil, err
//}
//share, err := s.LoadShare()
//if err != nil {
//return d, nil
//}
//d.share = share
//slog.Debugf("drand %s loaded", priv.Public.Address)
//return d, nil
//}

// StartDKG starts the DKG protocol by sending the first packet of the DKG
// protocol to every other node in the group. It returns nil if the DKG protocol
// finished successfully or an error otherwise.
func (d *Drand) StartDKG() error {
	d.dkg.Start()
	return d.WaitDKG()
}

// WaitDKG waits messages from the DKG protocol started by a leader or some
// nodes, and then wait until completion.
func (d *Drand) WaitDKG() error {
	var err error
	select {
	case share := <-d.dkg.WaitShare():
		s := key.Share(share)
		d.share = &s
	case err = <-d.dkg.WaitError():
	}
	if err != nil {
		return err
	}
	d.store.SaveShare(d.share)
	d.store.SaveDistPublic(d.share.Public())
	// XXX See if needed to change to qualified group
	d.store.SaveGroup(d.group)
	d.setDKGDone()
	return nil
}

// RandomBeacon starts periodically the TBLS protocol. The seed is the first
// message signed alongside with the current timestamp. All subsequent
// signatures are chained:
// s_i+1 = SIG(s_i || timestamp)
// For the moment, each resulting signature is stored in a file named
// beacons/<timestamp>.sig.
/*func (d *Drand) RandomBeacon(seed []byte, period time.Duration) {*/
//d.newBeacon().Start(seed, period)
//}

//func (d *Drand) newBeacon() *Beacon {
//d.state.Lock()
//defer d.state.Unlock()
//d.beacon = newBlsBeacon(d.share, d.group, d.r, d.store)
//return d.beacon
//}

//func (d *Drand) getBeacon() *Beacon {
//d.state.Lock()
//defer d.state.Unlock()
//return d.beacon
//}

func (d *Drand) Public(context.Context, *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	return nil, errors.New("not implemented yet")
}

func (d *Drand) Setup(c context.Context, in *dkg_proto.DKGPacket) (*dkg_proto.DKGResponse, error) {
	if d.isDKGDone() {
		return nil, errors.New("dkg finished already")
	}
	d.dkg.Process(c, in)
	return &dkg_proto.DKGResponse{}, nil
}

func (d *Drand) NewBeacon(c context.Context, in *drand.BeaconRequest) (*drand.BeaconResponse, error) {
	/* if drand.Beacon != nil {*/
	//beac := d.getBeacon()
	//if beac == nil {
	//slog.Debug("beacon not setup yet although receiving messages")
	//continue
	//}
	//beac.processBeaconPacket(pub, drand.Beacon)

	return nil, errors.New("not implemented yet")
}

// Loop waits infinitely and waits for incoming TBLS requests
func (d *Drand) Loop() {
	//d.newBeacon()
	<-d.done
}

func (d *Drand) Stop() {
	d.gateway.Stop()
	//d.beacon.Stop()
	close(d.done)
}

// isDKGDone returns true if the DKG protocol has already been executed. That
// means that the only packet that this node should receive are TBLS packet.
func (d *Drand) isDKGDone() bool {
	d.state.Lock()
	defer d.state.Unlock()
	return d.dkgDone
}

// setDKGDone marks the end of the "DKG" phase. After this call, Drand will only
// process TBLS packets.
func (d *Drand) setDKGDone() {
	d.state.Lock()
	defer d.state.Unlock()
	d.dkgDone = true
}

// little trick to be able to capture when drand is using the DKG methods,
// instead of offloading that to an external struct without any vision of drand
// internals, or implementing a big "Send" method directly on drand.
func (d *Drand) sendDkgPacket(p net.Peer, pack *dkg_proto.DKGPacket) error {
	_, err := d.gateway.Client.Setup(p, pack)
	return err
}

func (d *Drand) dkgNetwork() *dkgNetwork {
	return &dkgNetwork{d.sendDkgPacket}
}

type dkgNetwork struct {
	send func(net.Peer, *dkg_proto.DKGPacket) error
}

func (d *dkgNetwork) Send(p net.Peer, pack *dkg_proto.DKGPacket) error {
	return d.send(p, pack)
}

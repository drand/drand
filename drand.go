package main

import (
	"sync"
	"time"

	"github.com/nikkolasg/slog"

	kyber "gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/share/pedersen/dkg"
)

// Drand is the main logic of the program. It reads the keys / group file, it
// can start the DKG, read/write shars to files and can initiate/respond to TBlS
// signature requests.
type Drand struct {
	priv  *Private
	group *Group
	r     *Router

	dkg *DKG

	dks       *dkg.DistKeyShare // dkg private share. can be nil if dkg not executed.
	dkgDone   bool
	state     sync.Mutex
	shareFile string

	// the timestamp of the latest in-progress tbls signature
	currSig int64
	// whether the drand is actually expecting tbls message
	// or is waiting for the next period
	signing  bool
	sigState sync.Mutex // the mutex to hold the signing state
}

func NewDrandFromFile(privateFile, groupFile string) (*Drand, error) {
	priv := new(Private)
	if err := priv.Load(privateFile); err != nil {
		return nil, err
	}
	group := new(Group)
	if err := group.Load(groupFile); err != nil {
		return nil, err
	}
	return NewDrand(priv, group)
}

// NewDrandr initializes a fresh drandr. It loads the private / public identity
// and the group toml, and starts the router.
func NewDrand(priv *Private, group *Group) (*Drand, error) {
	router := NewRouter(priv, group)
	go router.Listen()
	dkg, err := NewDKG(priv, group, router)
	if err != nil {
		return nil, err
	}
	dr := &Drand{
		priv:  priv,
		group: group,
		r:     router,
		dkg:   dkg,
	}
	go dr.processMessages()
	return dr, nil
}

// LoadDrand intiliazes a drand with a distributed share already established.
func LoadDrand(privateFile, groupFile, shareFile string) (*Drand, error) {
	d, err := NewDrandFromFile(privateFile, groupFile)
	if err != nil {
		return nil, err
	}
	d.dks, err = LoadShare(shareFile)
	d.shareFile = shareFile
	d.dkgDone = true
	return d, err
}

// StartDKG starts the DKG protocol by sending the first packet of the DKG
// protocol to every other node in the group. It returns nil if the DKG protocol
// finished successfully or an error otherwise.
func (d *Drand) StartDKG(shareFile string) error {
	var err error
	d.dks, err = d.dkg.Start()
	if err != nil {
		return err
	}
	d.setDKGDone()
	return nil
}

// RunDKG runs the DKG protocol and saves the share to the given path.
// It returns nil if the DKG protocol finished successfully or an
// error otherwise.
func (d *Drand) RunDKG(shareFile string) error {
	var err error
	d.dks, err = d.dkg.Run()
	if err != nil {
		return err
	}
	d.setDKGDone()
	return nil
}

// RandomBeacon starts periodically the TBLS protocol. The seed is the first
// message signed alongside with the current timestamp. All subsequent
// signatures are chained:
// s_i+1 = SIG(s_i || timestamp)
// For the moment, each resulting signature is stored in a file named
// beacons/<timestamp>.sig.
func (d *Drand) RandomBeacon(seed []byte, period time.Duration) error {
	return nil
}

// Loop waits infinitely and waits for incoming TBLS requests
func (d *Drand) Loop() error {
	panic("not implemented yet")
}

func (d *Drand) processTBLS(pub *Public, msg *BeaconPacket) {

}

// processMessages runs in an infinite loop receiving message from the network
// and dispatching them to the dkg protocol or TBLS protocol depending on the
// state.
func (d *Drand) processMessages() {
	for {
		pub, buff := d.r.Receive()
		// if the dkg has not been finished yet, unmarshal with g2, otherwise
		// with g1.
		var g kyber.Group
		if d.isDKGDone() {
			g = g1
		} else {
			g = g2
		}
		drand, err := unmarshal(g, buff)
		if err != nil {
			slog.Debugf("%s: unmarshallable message from %s: %s", d.r.addr, pub.Address, err)
			continue
		}
		if drand.Beacon != nil {
			d.processTBLS(pub, drand.Beacon)
		} else if drand.Dkg != nil {
			d.dkg.process(pub, drand.Dkg)
		} else {
			slog.Debugf("%s: received weird message from %s", d.r.addr, pub.Address)
		}
	}
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

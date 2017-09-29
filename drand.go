package main

import (
	"sync"
	"time"

	"github.com/nikkolasg/slog"

	kyber "gopkg.in/dedis/kyber.v1"
)

// Drand is the main logic of the program. It reads the keys / group file, it
// can start the DKG, read/write shars to files and can initiate/respond to TBlS
// signature requests.
type Drand struct {
	priv   *Private
	group  *Group
	r      *Router
	store  Store
	dkg    *DKG
	beacon *Beacon

	share   *Share // dkg private share. can be nil if dkg not executed.
	dkgDone bool

	state sync.Mutex
	done  chan bool
}

// NewDrandFromConfig reads all the avaiable information from the config. It
// determines if the dkg is done or not.
func LoadDrand(s Store) (*Drand, error) {
	priv, err := s.LoadKey()
	if err != nil {
		return nil, err
	}
	group, err := s.LoadGroup()
	if err != nil {
		return nil, err
	}
	d, err := NewDrand(priv, group, s)
	if err != nil {
		return nil, err
	}
	share, err := s.LoadShare()
	if err != nil {
		return d, nil
	}
	d.share = share
	return d, nil
}

// XXX NewDrand is mostly used for testing purposes
func NewDrand(priv *Private, group *Group, s Store) (*Drand, error) {
	router := NewRouter(priv, group)
	go router.Listen()
	dkg, err := NewDKG(priv, group, router, s)
	if err != nil {
		return nil, err
	}
	dr := &Drand{
		priv:  priv,
		store: s,
		group: group,
		r:     router,
		dkg:   dkg,
		done:  make(chan bool),
	}
	go dr.processMessages()
	return dr, nil
}

// StartDKG starts the DKG protocol by sending the first packet of the DKG
// protocol to every other node in the group. It returns nil if the DKG protocol
// finished successfully or an error otherwise.
func (d *Drand) StartDKG() error {
	var err error
	d.share, err = d.dkg.Start()
	if err != nil {
		return err
	}
	d.store.SaveShare(d.share)
	d.setDKGDone()
	return nil
}

// RunDKG runs the DKG protocol and saves the share to the given path.
// It returns nil if the DKG protocol finished successfully or an
// error otherwise.
func (d *Drand) RunDKG() error {
	var err error
	d.share, err = d.dkg.Run()
	if err != nil {
		return err
	}
	d.store.SaveShare(d.share)
	d.setDKGDone()
	return nil
}

// RandomBeacon starts periodically the TBLS protocol. The seed is the first
// message signed alongside with the current timestamp. All subsequent
// signatures are chained:
// s_i+1 = SIG(s_i || timestamp)
// For the moment, each resulting signature is stored in a file named
// beacons/<timestamp>.sig.
func (d *Drand) RandomBeacon(seed []byte, period time.Duration) {
	d.newBeacon().Start(seed, period)
}

// Loop waits infinitely and waits for incoming TBLS requests
func (d *Drand) Loop() {
	d.newBeacon()
	<-d.done
}

func (d *Drand) Stop() {
	d.r.Stop()
	d.beacon.Stop()
	close(d.done)
}

func (d *Drand) newBeacon() *Beacon {
	d.state.Lock()
	defer d.state.Unlock()
	d.beacon = newBlsBeacon(d.share, d.group, d.r, d.store)
	return d.beacon
}

func (d *Drand) getBeacon() *Beacon {
	d.state.Lock()
	defer d.state.Unlock()
	return d.beacon
}

// processMessages runs in an infinite loop receiving message from the network
// and dispatching them to the dkg protocol or TBLS protocol depending on the
// state.
func (d *Drand) processMessages() {
	for {
		pub, buff := d.r.Receive()
		if pub == nil {
			slog.Debugf("drand %s leaving processing message", d.r.addr)
			return
		}
		slog.Debugf("drand %s: call router Receive() after from %s <>\n", d.r.addr, pub.Address)
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
			beac := d.getBeacon()
			if beac == nil {
				slog.Debug("beacon not setup yet although receiving messages")
				continue
			}
			beac.processBeaconPacket(pub, drand.Beacon)
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

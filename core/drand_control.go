package core

// drand_control.go contains the logic of the control interface of drand.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/dedis/drand/dkg"
	"github.com/dedis/drand/entropy"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/protobuf/drand"
	control "github.com/dedis/drand/protobuf/drand"
	vss "github.com/drand/kyber/share/vss/pedersen"
)

// InitDKG take a InitDKGPacket, extracts the informations needed and wait for the
// DKG protocol to finish. If the request specifies this node is a leader, it
// starts the DKG protocol.
func (d *Drand) InitDKG(c context.Context, in *control.InitDKGPacket) (*control.Empty, error) {
	d.state.Lock()

	if d.dkgDone == true {
		d.state.Unlock()
		return nil, errors.New("drand: dkg phase already done. Can't run 2 init DKG")
	}

	group, err := extractGroup(in.GetDkgGroup())
	if err != nil {
		d.state.Unlock()
		return nil, fmt.Errorf("drand: error reading group: %v", err)
	}
	d.group = group
	idx, found := group.Index(d.priv.Public)
	if !found {
		d.state.Unlock()
		return nil, errors.New("drand: public key not found in group")
	}
	d.idx = idx

	entropyReader := entropy.NewEntropyReader(in.Entropy, in.UserOnly)

	d.nextConf = &dkg.Config{
		Suite:          key.G2.(dkg.Suite),
		NewNodes:       d.group,
		Key:            d.priv,
		Reader:         entropyReader.ExtractReader(),
		UserReaderOnly: entropyReader.GetUserOnly(),
	}
	if err := setTimeout(d.nextConf, in.Timeout); err != nil {
		return nil, fmt.Errorf("drand: invalid timeout: %s", err)
	}

	d.state.Unlock()

	if in.GetIsLeader() {
		d.StartDKG()
	}
	if err := d.WaitDKG(); err != nil {
		return nil, fmt.Errorf("drand: err during DKG: %v", err)
	}

	//fmt.Printf("\n\n\ndrand %d -- %s: DKG finished. Starting beacon.\n\n\n", idx, d.priv.Public.Addr)
	d.initBeacon()
	time.Sleep(500 * time.Millisecond)
	// After DKG, always start the beacon directly
	if err := d.StartBeacon(false); err != nil {
		return nil, fmt.Errorf("drand: err during beacon generation: %v", err)
	}
	return &control.Empty{}, nil
}

// InitReshare receives information about the old and new group from which to
// operate the resharing protocol. It starts the resharing protocol if the
// received node is stated as a leader and is present in the old group.
// This function waits for the resharing DKG protocol to finish.
func (d *Drand) InitReshare(c context.Context, in *control.InitResharePacket) (*control.Empty, error) {
	var oldGroup, newGroup *key.Group
	var err error

	if newGroup, err = extractGroup(in.New); err != nil {
		return nil, err
	}

	d.state.Lock()
	if oldGroup, err = extractGroup(in.Old); err != nil {
		// try to get the current group
		if d.group == nil {
			d.state.Unlock()
			return nil, errors.New("drand: can't init-reshare if no old group provided")
		}
		d.log.With("module", "control").Debug("init_reshare", "old group equal current group")
		oldGroup = d.group
	}
	d.state.Unlock()

	oldIdx, oldPresent := oldGroup.Index(d.priv.Public)
	err = func() error {
		d.state.Lock()
		defer d.state.Unlock()

		if oldPresent {
			if d.group == nil {
				return errors.New("control: present in old group but no dkg here")
			}
			// stateful verification checking if we are in the old group that we have
			// and the one we receive
			currHash, err := d.group.Hash()
			if err != nil {
				return err
			}
			oldHash, err := oldGroup.Hash()
			if err != nil {
				return err
			}
			if currHash != oldHash {
				return errors.New("control: given old group is not the same as one saved")
			}
		}

		// prepare dkg config to run the protocol
		conf := &dkg.Config{
			OldNodes: oldGroup,
			NewNodes: newGroup,
			Key:      d.priv,
			Suite:    key.KeyGroup.(dkg.Suite),
		}

		// run the proto
		if oldPresent {
			if !d.dkgDone {
				return errors.New("control: can't reshare from old node when DKG not finished first")
			}
			if d.share == nil {
				return errors.New("control: can't reshare without a share")
			}
			conf.Share = d.share
		}

		nextHash, err := newGroup.Hash()
		if err != nil {
			return err
		}

		if err := setTimeout(conf, in.Timeout); err != nil {
			return fmt.Errorf("drand: invalid timeout: %s", err)
		}

		d.nextGroupHash = nextHash
		d.nextGroup = newGroup
		d.nextConf = conf
		d.nextOldPresent = oldPresent
		return nil
	}()

	if err != nil {
		return nil, err
	}

	if oldPresent && in.GetIsLeader() {
		// only the root sends a pre-message to the other old
		// nodes and start the DKG
		d.startResharingAsLeader(oldIdx)
	}
	if err := d.WaitDKG(); err != nil {
		return nil, err
	}
	// stop the beacon first, then re-create it with the new shares
	// i.e. the current beacon is still running alongside with the
	// new DKG but
	// stops as soon as the new DKG finishes.
	d.StopBeacon()
	d.initBeacon()
	time.Sleep(500 * time.Millisecond)
	catchup := true
	if oldPresent {
		catchup = false
	}
	return &control.Empty{}, d.StartBeacon(catchup)
}

func (d *Drand) startResharingAsLeader(oidx int) {
	d.log.With("module", "control").Debug("leader_reshare", "start signalling")
	d.state.Lock()
	msg := &control.ResharePacket{GroupHash: d.nextGroupHash}
	// send resharing packet to signal start of the protocol to other old
	// nodes
	for i, p := range d.nextConf.OldNodes.Identities() {
		if i == oidx {
			continue
		}
		id := p
		// XXX find way to just have a small RPC timeout if one is down.
		//fmt.Printf("drand leader %s -> signal to %s\n", d.priv.Public.Addr, id.Addr)
		if _, err := d.gateway.ProtocolClient.Reshare(id, msg); err != nil {
			//if _, err := d.gateway.InternalClient.Reshare(id, msg, grpc.FailFast(true)); err != nil {
			d.log.With("module", "control").Error("leader_reshare", err)
		}
	}
	d.state.Unlock()
	d.log.With("module", "control").Debug("leader_reshare", "start DKG")
	d.StartDKG()
}

// DistKey returns the distributed key corresponding to the current group
func (d *Drand) DistKey(context.Context, *drand.DistKeyRequest) (*drand.DistKeyResponse, error) {
	pt, err := d.store.LoadDistPublic()
	if err != nil {
		return nil, errors.New("drand: could not load dist. key")
	}
	buff, err := pt.Key().MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &drand.DistKeyResponse{
		Key: buff,
	}, nil
}

// PingPong simply responds with an empty packet, proving that this drand node
// is up and alive.
func (d *Drand) PingPong(c context.Context, in *control.Ping) (*control.Pong, error) {
	return &control.Pong{}, nil
}

// Share is a functionality of Control Service defined in protobuf/control that requests the private share of the drand node running locally
func (d *Drand) Share(ctx context.Context, in *control.ShareRequest) (*control.ShareResponse, error) {
	share, err := d.store.LoadShare()
	if err != nil {
		return nil, err
	}
	id := uint32(share.Share.I)
	buff, err := share.Share.V.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &control.ShareResponse{Index: id, Share: buff}, nil
}

// PublicKey is a functionality of Control Service defined in protobuf/control that requests the long term public key of the drand node running locally
func (d *Drand) PublicKey(ctx context.Context, in *control.PublicKeyRequest) (*control.PublicKeyResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()
	key, err := d.store.LoadKeyPair()
	if err != nil {
		return nil, err
	}
	protoKey, err := key.Public.Key.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &control.PublicKeyResponse{PubKey: protoKey}, nil
}

// PrivateKey is a functionality of Control Service defined in protobuf/control that requests the long term private key of the drand node running locally
func (d *Drand) PrivateKey(ctx context.Context, in *control.PrivateKeyRequest) (*control.PrivateKeyResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()
	key, err := d.store.LoadKeyPair()
	if err != nil {
		return nil, err
	}
	protoKey, err := key.Key.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &control.PrivateKeyResponse{PriKey: protoKey}, nil
}

// CollectiveKey replies with the distributed key in the response
func (d *Drand) CollectiveKey(ctx context.Context, in *control.CokeyRequest) (*control.CokeyResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()

	key, err := d.store.LoadDistPublic()
	if err != nil {
		return nil, err
	}
	protoKey, err := key.Key().MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &control.CokeyResponse{CoKey: protoKey}, nil
}

// GroupFile replies with the distributed key in the response
func (d *Drand) GroupFile(ctx context.Context, in *control.GroupTOMLRequest) (*control.GroupTOMLResponse, error) {
	d.state.Lock()
	defer d.state.Unlock()
	if d.group == nil {
		return nil, errors.New("drand: no dkg group setup yet")
	}
	gtoml := d.group.TOML()
	var buff bytes.Buffer
	err := toml.NewEncoder(&buff).Encode(gtoml)
	if err != nil {
		return nil, fmt.Errorf("drand: error encoding group to TOML: %s", err)
	}
	return &drand.GroupTOMLResponse{GroupToml: buff.String()}, nil
}

func extractGroup(i *control.GroupInfo) (*key.Group, error) {
	var g = &key.Group{}
	switch x := i.Location.(type) {
	case *control.GroupInfo_Path:
		// search group file via local filesystem path
		if err := key.Load(x.Path, g); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("control: can't allow new empty group")
	}
	// run a few checks on the proposed group
	if g.Len() < 4 {
		return nil, errors.New("control: can't accept group with fewer than 5 members")
	}
	if g.Threshold < vss.MinimumT(g.Len()) {
		return nil, errors.New("control: threshold of new group too low ")
	}
	return g, nil
}

func setTimeout(c *dkg.Config, timeoutStr string) error {
	// try parsing the timeout
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		if timeoutStr != "" {
			return fmt.Errorf("invalid timeout: %s", err)
		}
		timeout, _ = time.ParseDuration(DefaultDKGTimeout)
	}
	c.Timeout = timeout
	return nil
}

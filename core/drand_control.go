package core

// This file

import (
	"context"
	"errors"
	"fmt"

	"github.com/dedis/drand/dkg"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/protobuf/control"
	"github.com/dedis/drand/protobuf/crypto"
	dkg_proto "github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/kyber/share/vss/pedersen"
	"github.com/nikkolasg/slog"
)

// InitDKG take a DKGRequest, extracts the informations needed and wait for the
// DKG protocol to finish. If the request specify this node is a leader, it
// starts the DKG protocol.
func (d *Drand) InitDKG(c context.Context, in *control.DKGRequest) (*control.DKGResponse, error) {
	fmt.Println(" -- DRAND InitDKG CONTROL-- ")
	d.state.Lock()

	if d.dkgDone == true {
		return nil, errors.New("drand: dkg phase already done. Can't run 2 init DKG")
	}

	group, err := extractGroup(in.GetDkgGroup())
	if err != nil {
		d.state.Unlock()
		return nil, err
	}
	d.group = group
	if idx, found := group.Index(d.priv.Public); !found {
		d.state.Unlock()
		return nil, errors.New("drand: public key not found in group")
	} else {
		d.idx = idx
	}

	d.nextConf = &dkg.Config{
		Suite:     key.G2.(dkg.Suite),
		NewNodes:  d.group,
		Threshold: group.Threshold,
		Key:       d.priv,
	}

	d.state.Unlock()

	if in.GetIsLeader() {
		d.StartDKG()
	}
	if err := d.WaitDKG(); err != nil {
		return nil, err
	}
	return &control.DKGResponse{}, d.StartBeacon()
}

// Reshare receives information about the old and new group from which to
// operate the resharing protocol. It starts the resharing protocol if the
// received node is stated as a leader and is present in the old group.
// This function waits for the resharing DKG protocol to finish.
func (d *Drand) InitReshare(c context.Context, in *control.ReshareRequest) (*control.ReshareResponse, error) {
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
			} else {
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
		}

		// prepare dkg config to run the protocol
		conf := &dkg.Config{
			OldNodes:  oldGroup,
			NewNodes:  newGroup,
			Threshold: newGroup.Threshold,
			Key:       d.priv,
			Suite:     key.G2.(dkg.Suite),
		}
		// run the proto
		if oldPresent {
			if !d.dkgDone {
				return errors.New("control: can't reshare from old node when DKG not finished first")
			}
			if d.share == nil {
				return errors.New("control: can't reshare without a share !")
			}
			conf.Share = d.share
		}

		nextHash, err := newGroup.Hash()
		if err != nil {
			return err
		}

		d.nextGroupHash = nextHash
		d.nextGroup = newGroup
		d.nextConf = conf
		return nil
	}()

	if err != nil {
		return nil, err
	}

	if oldPresent && in.GetIsLeader() {
		// only the root sends a pre-message to the other old nodes and start
		// the DKG
		d.startResharingAsLeader(oldIdx)
	}
	if err := d.WaitDKG(); err != nil {
		return nil, err
	}

	return &control.ReshareResponse{}, d.StartBeacon()
}

func (d *Drand) startResharingAsLeader(oidx int) {
	d.state.Lock()
	msg := &dkg_proto.ResharePacket{GroupHash: d.nextGroupHash}
	// send resharing packet to signal start of the protocol to other old
	// nodes
	for i, p := range d.nextConf.OldNodes.Identities() {
		if i == oidx {
			continue
		}
		if _, err := d.gateway.InternalClient.Reshare(p, msg); err != nil {
			slog.Debugf("drand: init reshare packet err %s", err)
		}
	}
	d.state.Unlock()
	d.StartDKG()
}

func (d *Drand) DistKey(context.Context, *drand.DistKeyRequest) (*drand.DistKeyResponse, error) {
	pt, err := d.store.LoadDistPublic()
	if err != nil {
		return nil, errors.New("drand: could not load dist. key")
	}
	key, err := crypto.KyberToProtoPoint(pt.Key())
	if err != nil {
		slog.Fatal(err)
	}
	return &drand.DistKeyResponse{
		Key: key,
	}, nil
}

// GetShare is a functionality of Control Service defined in protobuf/control that requests the private share of the drand node running locally
func (d *Drand) GetShare(ctx context.Context, in *control.ShareRequest) (*control.ShareResponse, error) {
	share, err := d.store.LoadShare()
	if err != nil {
		slog.Fatal("drand: could not load the share")
	}
	id := uint32(share.Share.I)
	protoShare, err := crypto.KyberToProtoScalar(share.Share.V)
	if err != nil {
		slog.Fatal("drand: there is something wrong with the share")
	}
	return &control.ShareResponse{Index: id, Share: protoShare}, nil
}

func (d *Drand) PingPong(c context.Context, in *control.Ping) (*control.Pong, error) {
	return &control.Pong{}, nil
}

func extractGroup(i *control.GroupInfo) (*key.Group, error) {
	var g = &key.Group{}
	switch x := i.Location.(type) {
	case *control.GroupInfo_Path:
		// search group file via local filesystem path
		if err := key.Load(x.Path, g); err != nil {
			return nil, err
		}
	case nil:
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

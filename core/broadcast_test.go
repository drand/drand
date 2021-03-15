package core

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/util/random"
	"github.com/stretchr/testify/require"
)

type packInfo struct {
	id   string
	self *echoBroadcast
	p    *drand.DKGPacket
}

type callback func(*packInfo)

type callbackBroadcast struct {
	*echoBroadcast
	id string
	cb callback
}

func withCallback(id string, b *echoBroadcast, cb callback) Broadcast {
	return &callbackBroadcast{
		id:            id,
		echoBroadcast: b,
		cb:            cb,
	}
}

func (cb *callbackBroadcast) BroadcastDKG(c context.Context, p *drand.DKGPacket) (*drand.Empty, error) {
	r, err := cb.echoBroadcast.BroadcastDKG(c, p)
	if err != nil {
		return r, err
	}
	cb.cb(&packInfo{id: cb.id, self: cb.echoBroadcast, p: p})
	return r, err
}

func TestBroadcastSet(t *testing.T) {
	aset := new(arraySet)
	h1 := []byte("Hello")
	h2 := []byte("Hell2")
	aset.put(h1)
	require.True(t, aset.exists(h1))
	require.False(t, aset.exists(h2))
	aset.put(h1)
	require.True(t, aset.exists(h1))
	require.False(t, aset.exists(h2))
	aset.put(h2)
	require.True(t, aset.exists(h1))
	require.True(t, aset.exists(h2))
}

func TestBroadcast(t *testing.T) {
	n := 5
	drands, group, dir, _ := BatchNewDrand(n, true)
	defer os.RemoveAll(dir)
	defer CloseAllDrands(drands)

	// channel that will receive all broadcasted packets
	incPackets := make(chan *packInfo)
	// callback that all nodes execute when they receive a "successful" packet
	callback := func(p *packInfo) {
		incPackets <- p
	}
	broads := make([]*echoBroadcast, 0, n)
	ids := make([]string, 0, n)
	for _, d := range drands {
		id := d.priv.Public.Address()
		b := newEchoBroadcast(d.log, d.privGateway.ProtocolClient, id, group.Nodes, func(dkg.Packet) error { return nil })

		d.dkgInfo = &dkgInfo{
			board:   withCallback(id, b, callback),
			started: true,
		}
		broads = append(broads, b)
		ids = append(ids, id)
	}

	deal := fakeDeal()
	dealProto, err := dkgPacketToProto(deal)
	require.NoError(t, err)
	_, err = broads[0].BroadcastDKG(context.Background(), &drand.DKGPacket{Dkg: dealProto})
	require.NoError(t, err)
	received := make(map[string]bool)
	exp := n * (n - 1)
	for i := 0; i < exp; i++ {
		select {
		case info := <-incPackets:
			info.self.Lock()
			require.True(t, info.self.hashes.exists(deal.Hash()))
			require.True(t, len(info.self.dealCh) == 1, "len of channel is %d", len(info.self.dealCh))
			received[info.id] = true
			info.self.Unlock()
		case <-time.After(5 * time.Second):
			require.True(t, false, "test failed to continue")
		}
	}
	for _, id := range ids {
		require.True(t, received[id])
	}
	for _, b := range broads {
		drain(t, b.dealCh)
	}

	// try again to broadcast but it shouldn't actually do it because the first
	// node (the one we ask to send first) already has the hash registered.
	_, err = broads[0].BroadcastDKG(context.Background(), &drand.DKGPacket{Dkg: dealProto})
	require.NoError(t, err)
	select {
	case <-incPackets:
		require.False(t, true, "deal shouldn't be passed down to application")
	case <-time.After(500 * time.Millisecond):
		require.Len(t, broads[0].dealCh, 0)
	}

	// let's make everyone broadcast a different packet
	hashes := make([][]byte, 0, n-1)
	for _, b := range broads[1:] {
		deal := fakeDeal()
		dealProto, err := dkgPacketToProto(deal)
		require.NoError(t, err)
		_, err = b.BroadcastDKG(context.Background(), &drand.DKGPacket{
			Dkg: dealProto,
		})
		require.NoError(t, err)
		hashes = append(hashes, deal.Hash())
	}

	received = make(map[string]bool)
	exp = exp * (n - 1)
	for i := 0; i < exp; i++ {
		select {
		case info := <-incPackets:
			info.self.Lock()
			require.True(t, info.self.hashes.exists(deal.Hash()))
			received[info.id] = true
			info.self.Unlock()
		case <-time.After(5 * time.Second):
			require.True(t, false, "test failed to continue")
		}
	}
	// check if everyone received
	for _, id := range ids {
		require.True(t, received[id])
	}
	// check if they all have all hashes
	for _, broad := range broads {
		for _, hash := range hashes {
			require.True(t, broad.hashes.exists(hash))
		}
	}

	// check that it dispatches to the correct channel
	broads[0].passToApplication(&dkg.ResponseBundle{})
	require.True(t, len(broads[0].respCh) == 1)
	broads[0].passToApplication(&dkg.JustificationBundle{})
	require.True(t, len(broads[0].justCh) == 1)
}

func drain(t *testing.T, ch chan dkg.DealBundle) int {
	t.Helper()
	var howMany int
	for {
		select {
		case <-ch:
			howMany++
		case <-time.After(100 * time.Millisecond):
			return howMany
		}
	}
}

func fakeDeal() *dkg.DealBundle {
	return &dkg.DealBundle{
		DealerIndex: 0,
		Public:      []kyber.Point{key.KeyGroup.Point().Pick(random.New())},
		Deals: []dkg.Deal{{
			ShareIndex:     1,
			EncryptedShare: []byte("HelloWorld"),
		}},
	}
}

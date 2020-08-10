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

	broads := make([]*broadcast, 0, n)
	for _, d := range drands {
		b := newBroadcast(d.log, d.privGateway.ProtocolClient, d.priv.Public.Address(), group.Nodes, func(dkg.Packet) error { return nil })
		d.dkgInfo = &dkgInfo{
			board:   b,
			started: true,
		}
		broads = append(broads, b)
	}

	deal := fakeDeal()
	dealProto, err := dkgPacketToProto(deal)
	require.NoError(t, err)
	_, err = broads[0].BroadcastDKG(context.Background(), &drand.DKGPacket{Dkg: dealProto})
	require.NoError(t, err)
	// leave some time so other get it
	time.Sleep(100 * time.Millisecond)
	for _, b := range broads {
		b.Lock()
		require.True(t, b.hashes.exists(deal.Hash()))
		require.True(t, len(b.dealCh) == 1, "len of channel is %d", len(b.dealCh))
		// drain the channel
		<-b.dealCh
		b.Unlock()
	}

	// try again to broadcast but it shouldn't actually do it
	broads[1].Lock()
	broads[1].hashes = new(arraySet)
	broads[1].Unlock()
	_, err = broads[0].BroadcastDKG(context.Background(), &drand.DKGPacket{Dkg: dealProto})
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)
	broads[1].Lock()
	// so here it shouldnt have the entry since we deleted it
	// make sure first that the channel is empty
	require.Len(t, broads[1].dealCh, 0)
	select {
	case <-broads[1].dealCh:
		require.False(t, true, "deal shouldn't be passed down to application")
	case <-time.After(500 * time.Millisecond):
		// all good
	}
	// put it again
	broads[1].hashes.put(deal.Hash())
	broads[1].Unlock()

	// let's make everyone broadcast a different packet
	for _, b := range broads[1:] {
		deal := fakeDeal()
		dealProto, err := dkgPacketToProto(deal)
		require.NoError(t, err)
		_, err = b.BroadcastDKG(context.Background(), &drand.DKGPacket{
			Dkg: dealProto,
		})
		require.NoError(t, err)
	}

	time.Sleep(100 * time.Millisecond)
	for i, b := range broads {
		require.Equal(t, drain(t, b.dealCh), n-1, "node %d failed", i)
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

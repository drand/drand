package beacon

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"

	clock "github.com/jonboulle/clockwork"
)

func BenchmarkCatchup(b *testing.B) {
	dbFolder, err := ioutil.TempDir("", "catchup")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dbFolder)

	store, err := boltdb.NewBoltStore(dbFolder, nil)
	if err != nil {
		b.Fatal(err)
	}

	shares, commits := dkgShares(1, 1)
	_, group := test.BatchIdentities(1)
	group.Threshold = 1
	group.Period = time.Second
	group.GenesisTime = time.Now().Add(time.Duration(-1*b.N) * time.Second).UnixNano()
	group.PublicKey = &key.DistPublic{Coefficients: commits}

	conf := &Config{
		Public: nil,
		Group:  group,
		Share:  nil,
		Clock:  clock.NewFakeClock(),
	}

	// generate b.N beacons
	beacons := make([]chain.Beacon, 0, b.N)
	beacons = append(beacons, chain.Beacon{Round: 0, Signature: group.GetGenesisSeed()})
	for i := 1; i < b.N; i++ {
		b := chain.Beacon{Round: uint64(i), PreviousSig: beacons[i-1].Signature}
		partial, _ := key.Scheme.Sign(shares[0].PrivateShare(), b.PreviousSig)
		b.Signature, _ = key.Scheme.Recover(group.PublicKey.PubPoly(), b.PreviousSig, [][]byte{partial}, 1, 1)
		beacons = append(beacons, b)
	}
	// don't send the genesis round
	beacons = beacons[1:]

	fakeServer := FakePC{make(chan *drand.BeaconPacket, 1)}
	h, err := NewHandler(fakeServer, store, conf, log.DefaultLogger())
	if err != nil {
		b.Fatal(err)
	}

	go func() {
		for _, b := range beacons {
			fakeServer.beaconChan <- beaconToProto(&b)
		}
		close(fakeServer.beaconChan)
	}()

	b.ResetTimer()
	h.chain.RunSync(context.Background(), 0, []net.Peer{})
	bea, err := h.chain.Last()
	if err != nil {
		b.Fatal(err)
	}
	if bea.GetRound() != uint64(b.N) {
		b.Fatal(fmt.Errorf("wrong round %d vs %d", bea.GetRound(), b.N))
	}
}

type FakePC struct {
	beaconChan chan *drand.BeaconPacket
}

func (f FakePC) GetIdentity(ctx context.Context, p net.Peer, in *drand.IdentityRequest, opts ...net.CallOption) (*drand.Identity, error) {
	return nil, nil
}
func (f FakePC) SyncChain(ctx context.Context, p net.Peer, in *drand.SyncRequest, opts ...net.CallOption) (chan *drand.BeaconPacket, error) {
	return f.beaconChan, nil
}
func (f FakePC) PartialBeacon(ctx context.Context, p net.Peer, in *drand.PartialBeaconPacket, opts ...net.CallOption) error {
	return nil
}
func (f FakePC) FreshDKG(ctx context.Context, p net.Peer, in *drand.DKGPacket, opts ...net.CallOption) (*drand.Empty, error) {
	return nil, nil
}
func (f FakePC) ReshareDKG(ctx context.Context, p net.Peer, in *drand.ResharePacket, opts ...net.CallOption) (*drand.Empty, error) {
	return nil, nil
}
func (f FakePC) SignalDKGParticipant(ctx context.Context, p net.Peer, in *drand.SignalDKGPacket, opts ...net.CallOption) error {
	return nil
}
func (f FakePC) PushDKGInfo(ctx context.Context, p net.Peer, in *drand.DKGInfoPacket, opts ...net.CallOption) error {
	return nil
}
func (f FakePC) SetTimeout(time.Duration) {}

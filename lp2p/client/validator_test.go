package client

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/test/cache"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"google.golang.org/protobuf/proto"
)

type randomDataWrapper struct {
	data client.RandomData
}

func (r *randomDataWrapper) Round() uint64 {
	return r.data.Rnd
}

func (r *randomDataWrapper) Signature() []byte {
	return r.data.Sig
}

func (r *randomDataWrapper) Randomness() []byte {
	return r.data.Random
}

func randomPeerID(t *testing.T) peer.ID {
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return peerID
}

func fakeRandomData(info *chain.Info) client.RandomData {
	rnd := chain.CurrentRound(time.Now().Unix(), info.Period, info.GenesisTime)

	sig := make([]byte, 8)
	binary.LittleEndian.PutUint64(sig, rnd)
	psig := make([]byte, 8)
	binary.LittleEndian.PutUint64(psig, rnd-1)

	return client.RandomData{
		Rnd:               rnd,
		Sig:               sig,
		PreviousSignature: psig,
		Random:            chain.RandomnessFromSignature(sig),
	}
}

func fakeChainInfo() *chain.Info {
	return &chain.Info{
		Period:      time.Second,
		GenesisTime: time.Now().Unix(),
		PublicKey:   test.GenerateIDs(1)[0].Public.Key,
	}
}

func TestRejectsUnmarshalBeaconFailure(t *testing.T) {
	c := Client{log: log.DefaultLogger()}
	validate := randomnessValidator(fakeChainInfo(), nil, &c)

	msg := pubsub.Message{Message: &pb.Message{}}
	res := validate(context.Background(), randomPeerID(t), &msg)

	if res != pubsub.ValidationReject {
		t.Fatal(errors.New("expected reject for invalid message"))
	}
}

func TestAcceptsWithoutTrustRoot(t *testing.T) {
	c := Client{log: log.DefaultLogger()}
	validate := randomnessValidator(nil, nil, &c)

	resp := drand.PublicRandResponse{}
	data, err := proto.Marshal(&resp)
	if err != nil {
		t.Fatal(err)
	}
	msg := pubsub.Message{Message: &pb.Message{Data: data}}
	res := validate(context.Background(), randomPeerID(t), &msg)

	if res != pubsub.ValidationAccept {
		t.Fatal(errors.New("expected accept without trust root"))
	}
}

func TestRejectsFutureBeacons(t *testing.T) {
	info := fakeChainInfo()
	c := Client{log: log.DefaultLogger()}
	validate := randomnessValidator(info, nil, &c)

	resp := drand.PublicRandResponse{
		Round: chain.CurrentRound(time.Now().Unix(), info.Period, info.GenesisTime) + 5,
	}
	data, err := proto.Marshal(&resp)
	if err != nil {
		t.Fatal(err)
	}
	msg := pubsub.Message{Message: &pb.Message{Data: data}}
	res := validate(context.Background(), randomPeerID(t), &msg)

	if res != pubsub.ValidationReject {
		t.Fatal(errors.New("expected reject for future message"))
	}
}

func TestRejectsVerifyBeaconFailure(t *testing.T) {
	info := fakeChainInfo()
	c := Client{log: log.DefaultLogger()}
	validate := randomnessValidator(info, nil, &c)

	resp := drand.PublicRandResponse{
		Round: chain.CurrentRound(time.Now().Unix(), info.Period, info.GenesisTime),
		// missing signature etc.
	}
	data, err := proto.Marshal(&resp)
	if err != nil {
		t.Fatal(err)
	}
	msg := pubsub.Message{Message: &pb.Message{Data: data}}
	res := validate(context.Background(), randomPeerID(t), &msg)

	if res != pubsub.ValidationReject {
		t.Fatal(errors.New("expected reject for beacon verification failure"))
	}
}

func TestIgnoresCachedEqualBeacon(t *testing.T) {
	info := fakeChainInfo()
	ca := cache.NewMapCache()
	c := Client{log: log.DefaultLogger()}
	validate := randomnessValidator(info, ca, &c)
	rdata := fakeRandomData(info)

	ca.Add(rdata.Rnd, &rdata)

	resp := drand.PublicRandResponse{
		Round:             rdata.Rnd,
		Signature:         rdata.Sig,
		PreviousSignature: rdata.PreviousSignature,
		Randomness:        rdata.Random,
	}
	data, err := proto.Marshal(&resp)
	if err != nil {
		t.Fatal(err)
	}
	msg := pubsub.Message{Message: &pb.Message{Data: data}}
	res := validate(context.Background(), randomPeerID(t), &msg)

	if res != pubsub.ValidationIgnore {
		t.Fatal(errors.New("expected ignore for cached beacon"))
	}
}

func TestRejectsCachedUnequalBeacon(t *testing.T) {
	info := fakeChainInfo()
	ca := cache.NewMapCache()
	c := Client{log: log.DefaultLogger()}
	validate := randomnessValidator(info, ca, &c)
	rdata := fakeRandomData(info)

	ca.Add(rdata.Rnd, &rdata)

	sig := make([]byte, 8)
	binary.LittleEndian.PutUint64(sig, rdata.Rnd+1)

	resp := drand.PublicRandResponse{
		Round:             rdata.Rnd,
		Signature:         rdata.Sig,
		PreviousSignature: sig, // incoming message has incorrect previous sig
		Randomness:        rdata.Random,
	}
	data, err := proto.Marshal(&resp)
	if err != nil {
		t.Fatal(err)
	}
	msg := pubsub.Message{Message: &pb.Message{Data: data}}
	res := validate(context.Background(), randomPeerID(t), &msg)

	if res != pubsub.ValidationReject {
		t.Fatal(errors.New("expected reject for cached but unequal beacon"))
	}
}

func TestIgnoresCachedEqualNonRandomDataBeacon(t *testing.T) {
	info := fakeChainInfo()
	ca := cache.NewMapCache()
	c := Client{log: log.DefaultLogger()}
	validate := randomnessValidator(info, ca, &c)
	rdata := randomDataWrapper{fakeRandomData(info)}

	ca.Add(rdata.Round(), &rdata)

	resp := drand.PublicRandResponse{
		Round:             rdata.Round(),
		Signature:         rdata.Signature(),
		PreviousSignature: rdata.data.PreviousSignature,
		Randomness:        rdata.Randomness(),
	}
	data, err := proto.Marshal(&resp)
	if err != nil {
		t.Fatal(err)
	}
	msg := pubsub.Message{Message: &pb.Message{Data: data}}
	res := validate(context.Background(), randomPeerID(t), &msg)

	if res != pubsub.ValidationIgnore {
		t.Fatal(errors.New("expected ignore for cached beacon"))
	}
}

func TestRejectsCachedEqualNonRandomDataBeacon(t *testing.T) {
	info := fakeChainInfo()
	ca := cache.NewMapCache()
	c := Client{log: log.DefaultLogger()}
	validate := randomnessValidator(info, ca, &c)
	rdata := randomDataWrapper{fakeRandomData(info)}

	ca.Add(rdata.Round(), &rdata)

	sig := make([]byte, 8)
	binary.LittleEndian.PutUint64(sig, rdata.Round()+1)

	resp := drand.PublicRandResponse{
		Round:             rdata.Round(),
		Signature:         sig, // incoming message has incorrect sig
		PreviousSignature: rdata.data.PreviousSignature,
		Randomness:        rdata.Randomness(),
	}
	data, err := proto.Marshal(&resp)
	if err != nil {
		t.Fatal(err)
	}
	msg := pubsub.Message{Message: &pb.Message{Data: data}}
	res := validate(context.Background(), randomPeerID(t), &msg)

	if res != pubsub.ValidationReject {
		t.Fatal(errors.New("expected reject for cached beacon"))
	}
}

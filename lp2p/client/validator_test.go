package client

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/drand/drand/chain"
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

func TestRejectInvalid(t *testing.T) {
	info := chain.Info{
		Period:      time.Second,
		GenesisTime: time.Now().Unix(),
		PublicKey:   test.GenerateIDs(1)[0].Public.Key,
	}
	cache := cache.NewMapCache()
	c := Client{log: log.DefaultLogger()}
	validate := randomnessValidator(&info, cache, &c)

	msg := pubsub.Message{Message: &pb.Message{}}
	res := validate(context.Background(), randomPeerID(t), &msg)

	if res != pubsub.ValidationReject {
		t.Fatal(errors.New("expected reject for invalid message"))
	}
}

func TestAcceptWithoutTrustRoot(t *testing.T) {
	cache := cache.NewMapCache()
	c := Client{log: log.DefaultLogger()}
	validate := randomnessValidator(nil, cache, &c)

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

func TestRejectFuture(t *testing.T) {
	info := chain.Info{
		Period:      time.Second,
		GenesisTime: time.Now().Unix(),
		PublicKey:   test.GenerateIDs(1)[0].Public.Key,
	}
	cache := cache.NewMapCache()
	c := Client{log: log.DefaultLogger()}
	validate := randomnessValidator(&info, cache, &c)

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

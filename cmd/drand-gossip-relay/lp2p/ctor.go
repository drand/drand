package lp2p

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	routing "github.com/libp2p/go-libp2p-routing"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	xerrors "golang.org/x/xerrors"
)

var (
	log       = logging.Logger("lp2p")
	privDsKey = datastore.NewKey("p2pPrivKey")
)

func PubSubTopic(nn string) string {
	return fmt.Sprintf("/drand/pubsub/v0.0.0/%s", nn)
}

func ConstructHost(priv crypto.PrivKey, listenAddr string) (host.Host, *dht.IpfsDHT, error) {
	var idht *dht.IpfsDHT

	ctx := context.TODO()
	h, err := libp2p.New(ctx,
		libp2p.ListenAddrStrings(listenAddr),
		libp2p.Identity(priv),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			var err error
			idht, err = dht.New(ctx, h)
			return idht, err
		}),
		libp2p.DisableRelay(),
	)
	if err != nil {
		return nil, nil, xerrors.Errorf("constructing host: %w", err)
	}
	return h, idht, nil
}

func ConstructPubSub(h host.Host) (*pubsub.PubSub, error) {
	p, err := pubsub.NewGossipSub(context.TODO(), h)
	if err != nil {
		return nil, xerrors.Errorf("constructing pubsub: %d", err)
	}

	return p, nil
}

func LoadOrCreatePrivKey(ds datastore.Datastore) (crypto.PrivKey, error) {
	privBytes, err := ds.Get(privDsKey)

	var priv crypto.PrivKey
	switch {
	case err == nil:
		priv, err = crypto.UnmarshalEd25519PrivateKey(privBytes)

		if err != nil {
			return nil, xerrors.Errorf("decoding ed25519 key: %w", err)
		}
		log.Infof("loaded private key")

	case xerrors.Is(err, datastore.ErrNotFound):
		priv, _, err = crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, xerrors.Errorf("generating private key: %w", err)
		}
	default:
		return nil, xerrors.Errorf("getting private key: %w", err)
	}

	return priv, nil
}

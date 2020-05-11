package lp2p

import (
	"context"
	"crypto/rand"
	"fmt"
	mrand "math/rand"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	record "github.com/libp2p/go-libp2p-record"
	routing "github.com/libp2p/go-libp2p-routing"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	ma "github.com/multiformats/go-multiaddr"
	xerrors "golang.org/x/xerrors"
)

var (
	log       = logging.Logger("lp2p")
	privDsKey = datastore.NewKey("p2pPrivKey")
)

func PubSubTopic(nn string) string {
	return fmt.Sprintf("/drand/pubsub/v0.0.0/%s", nn)
}

func ConstructHost(ds datastore.Datastore, priv crypto.PrivKey, listenAddr string, bootstrap []ma.Multiaddr) (host.Host, *dht.IpfsDHT, *pubsub.PubSub, error) {
	var idht *dht.IpfsDHT
	dhtDs := namespace.Wrap(ds, datastore.NewKey("/dht"))

	addrInfos, err := peer.AddrInfosFromP2pAddrs(bootstrap...)
	if err != nil {
		fmt.Printf("%+v", bootstrap)
		return nil, nil, nil, xerrors.Errorf("parsing addrInfos: %+v", err)
	}

	ctx := context.TODO()
	h, err := libp2p.New(ctx,
		libp2p.ListenAddrStrings(listenAddr),
		libp2p.Identity(priv),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			var err error
			idht, err = dht.New(ctx, h,
				dht.Mode(dht.ModeServer),
				dht.Datastore(dhtDs),
				dht.Validator(record.NamespacedValidator{
					"pk": record.PublicKeyValidator{},
				}),
				dht.QueryFilter(dht.PublicQueryFilter),
				dht.RoutingTableFilter(dht.PublicRoutingTableFilter),
				dht.DisableProviders(),
				dht.DisableValues(),
				dht.BootstrapPeers(bootstrap...),
				dht.ProtocolPrefix("/drand/"),
			)
			return idht, err
		}),
		libp2p.DisableRelay(),
	)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("constructing host: %w", err)
	}

	p, err := pubsub.NewGossipSub(context.TODO(), h)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("constructing pubsub: %d", err)
	}

	go func() {
		mrand.Shuffle(len(addrInfos), func(i, j int) {
			addrInfos[i], addrInfos[j] = addrInfos[j], addrInfos[i]
		})
		for _, ai := range addrInfos {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := h.Connect(ctx, ai)
			cancel()
			if err != nil {
				log.Warnf("could not bootstrap with: %s", ai)
			}
		}
	}()
	return h, idht, p, nil
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

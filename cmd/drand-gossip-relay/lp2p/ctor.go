package lp2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	mrand "math/rand"
	"os"
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
	log      = logging.Logger("lp2p")
	privFile = "identity.key"
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

	ctx := context.Background()
	h, err := libp2p.New(ctx,
		libp2p.ListenAddrStrings(listenAddr),
		libp2p.Identity(priv),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			var err error
			idht, err = dht.New(ctx, h,
				dht.Mode(dht.ModeAutoServer),
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

	p, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("constructing pubsub: %d", err)
	}

	go func() {
		mrand.Shuffle(len(addrInfos), func(i, j int) {
			addrInfos[i], addrInfos[j] = addrInfos[j], addrInfos[i]
		})
		for _, ai := range addrInfos {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
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
	privBytes, err := ioutil.ReadFile(privFile)

	var priv crypto.PrivKey
	switch {
	case err == nil:
		priv, err = crypto.UnmarshalEd25519PrivateKey(privBytes)

		if err != nil {
			return nil, xerrors.Errorf("decoding ed25519 key: %w", err)
		}
		log.Infof("loaded private key")

	case xerrors.Is(err, os.ErrNotExist):
		priv, _, err = crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, xerrors.Errorf("generating private key: %w", err)
		}
		b, err := priv.Bytes()
		if err != nil {
			return nil, xerrors.Errorf("marshaling private key: %w", err)
		}
		err = ioutil.WriteFile(privFile, b, 0600)
		if err != nil {
			return nil, xerrors.Errorf("writing identity fiel: %w", err)
		}

	default:
		return nil, xerrors.Errorf("getting private key: %w", err)
	}

	return priv, nil
}

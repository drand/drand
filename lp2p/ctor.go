package lp2p

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	mrand "math/rand"
	"os"
	"path"
	"time"

	dlog "github.com/drand/drand/log"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	noise "github.com/libp2p/go-libp2p-noise"
	"github.com/libp2p/go-libp2p-peerstore/pstoreds"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/crypto/blake2b"
	xerrors "golang.org/x/xerrors"
)

const (
	// userAgent sets the libp2p user-agent which is sent along with the identify protocol.
	userAgent = "drand-relay/0.0.0"
	// directConnectTicks makes pubsub check it's connected to direct peers every N seconds.
	directConnectTicks uint64 = 5
	lowWater                  = 50
	highWater                 = 200
	gracePeriod               = time.Minute
	bootstrapTimeout          = 5 * time.Second
)

// PubSubTopic generates a drand pubsub topic from a chain hash.
func PubSubTopic(h string) string {
	return fmt.Sprintf("/drand/pubsub/v0.0.0/%s", h)
}

// ConstructHost build a libp2p host configured for relaying drand randomness over pubsub.
func ConstructHost(ds datastore.Datastore, priv crypto.PrivKey, listenAddr string,
	bootstrap []ma.Multiaddr, log dlog.Logger) (host.Host, *pubsub.PubSub, error) {
	ctx := context.Background()

	pstoreDs := namespace.Wrap(ds, datastore.NewKey("/peerstore"))
	pstore, err := pstoreds.NewPeerstore(ctx, pstoreDs, pstoreds.DefaultOpts())
	if err != nil {
		return nil, nil, xerrors.Errorf("creating peerstore: %w", err)
	}
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		return nil, nil, xerrors.Errorf("computing peerid: %w", err)
	}
	err = pstore.AddPrivKey(peerID, priv)
	if err != nil {
		return nil, nil, xerrors.Errorf("adding priv to keystore: %w", err)
	}

	addrInfos, err := resolveAddresses(ctx, bootstrap, nil)
	if err != nil {
		return nil, nil, xerrors.Errorf("parsing addrInfos: %+v", err)
	}

	cmgr := connmgr.NewConnManager(lowWater, highWater, gracePeriod)

	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ChainOptions(
			libp2p.Security(libp2ptls.ID, libp2ptls.New),
			libp2p.Security(noise.ID, noise.New)),
		libp2p.DisableRelay(),
		// libp2p.Peerstore(pstore), depends on https://github.com/libp2p/go-libp2p-peerstore/issues/153
		libp2p.UserAgent(userAgent),
		libp2p.ConnectionManager(cmgr),
	}

	if listenAddr != "" {
		opts = append(opts, libp2p.ListenAddrStrings(listenAddr))
	} else {
		opts = append(opts, libp2p.NoListenAddrs)
	}

	h, err := libp2p.New(ctx, opts...)
	if err != nil {
		return nil, nil, xerrors.Errorf("constructing host: %w", err)
	}

	p, err := pubsub.NewGossipSub(ctx, h,
		pubsub.WithPeerExchange(true),
		pubsub.WithMessageIdFn(func(pmsg *pubsubpb.Message) string {
			hash := blake2b.Sum256(pmsg.Data)
			return string(hash[:])
		}),
		pubsub.WithDirectPeers(addrInfos),
		pubsub.WithFloodPublish(true),
		pubsub.WithDirectConnectTicks(directConnectTicks),
	)
	if err != nil {
		return nil, nil, xerrors.Errorf("constructing pubsub: %d", err)
	}

	go func() {
		mrand.Shuffle(len(addrInfos), func(i, j int) {
			addrInfos[i], addrInfos[j] = addrInfos[j], addrInfos[i]
		})
		for _, ai := range addrInfos {
			ctx, cancel := context.WithTimeout(ctx, bootstrapTimeout)
			err := h.Connect(ctx, ai)
			cancel()
			if err != nil {
				log.Warn("construct_host", "could not bootstrap", "addr", ai)
			}
		}
	}()
	return h, p, nil
}

// LoadOrCreatePrivKey loads a base64 encoded libp2p private key from a file or creates one if it does not exist.
func LoadOrCreatePrivKey(identityPath string, log dlog.Logger) (crypto.PrivKey, error) {
	privB64, err := ioutil.ReadFile(identityPath)

	var priv crypto.PrivKey
	switch {
	case err == nil:
		privBytes, err := base64.RawStdEncoding.DecodeString(string(privB64))
		if err != nil {
			return nil, xerrors.Errorf("decoding base64 key: %w", err)
		}
		priv, err = crypto.UnmarshalEd25519PrivateKey(privBytes)
		if err != nil {
			return nil, xerrors.Errorf("unmarshaling ed25519 key: %w", err)
		}
		log.Info("load_or_create_priv_key", "loaded private key")

	case xerrors.Is(err, os.ErrNotExist):
		priv, _, err = crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, xerrors.Errorf("generating private key: %w", err)
		}
		b, err := priv.Raw()
		if err != nil {
			return nil, xerrors.Errorf("marshaling private key: %w", err)
		}
		err = os.MkdirAll(path.Dir(identityPath), 0755)
		if err != nil {
			return nil, xerrors.Errorf("creating identity directory and parents: %w", err)
		}
		err = ioutil.WriteFile(identityPath, []byte(base64.RawStdEncoding.EncodeToString(b)), 0600)
		if err != nil {
			return nil, xerrors.Errorf("writing identity file: %w", err)
		}

	default:
		return nil, xerrors.Errorf("getting private key: %w", err)
	}

	return priv, nil
}

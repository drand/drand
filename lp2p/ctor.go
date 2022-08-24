package lp2p

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	mrand "math/rand"
	"os"
	"path"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoreds"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"

	dlog "github.com/drand/drand/log"
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
	allDirPerm                = 0755
	identityFilePerm          = 0600
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
		return nil, nil, fmt.Errorf("creating peerstore: %w", err)
	}
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("computing peerid: %w", err)
	}
	err = pstore.AddPrivKey(peerID, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("adding priv to keystore: %w", err)
	}

	addrInfos, err := resolveAddresses(ctx, bootstrap, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing addrInfos: %w", err)
	}

	cmgr, err := connmgr.NewConnManager(lowWater, highWater, connmgr.WithGracePeriod(gracePeriod))
	if err != nil {
		return nil, nil, fmt.Errorf("constructing connmanager: %w", err)
	}

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

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("constructing host: %w", err)
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
		return nil, nil, fmt.Errorf("constructing pubsub: %w", err)
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
				log.Warnw("", "construct_host", "could not bootstrap", "addr", ai)
			}
		}
	}()
	return h, p, nil
}

// LoadOrCreatePrivKey loads a base64 encoded libp2p private key from a file or creates one if it does not exist.
func LoadOrCreatePrivKey(identityPath string, log dlog.Logger) (crypto.PrivKey, error) {
	privB64, err := os.ReadFile(identityPath)

	var priv crypto.PrivKey
	switch {
	case err == nil:
		privBytes, err := base64.RawStdEncoding.DecodeString(string(privB64))
		if err != nil {
			return nil, fmt.Errorf("decoding base64 key: %w", err)
		}
		priv, err = crypto.UnmarshalEd25519PrivateKey(privBytes)
		if err != nil {
			return nil, fmt.Errorf("unmarshaling ed25519 key: %w", err)
		}
		log.Infow("", "load_or_create_priv_key", "loaded private key")

	case errors.Is(err, os.ErrNotExist):
		priv, _, err = crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generating private key: %w", err)
		}
		b, err := priv.Raw()
		if err != nil {
			return nil, fmt.Errorf("marshaling private key: %w", err)
		}
		err = os.MkdirAll(path.Dir(identityPath), allDirPerm)
		if err != nil {
			return nil, fmt.Errorf("creating identity directory and parents: %w", err)
		}
		err = os.WriteFile(identityPath, []byte(base64.RawStdEncoding.EncodeToString(b)), identityFilePerm)
		if err != nil {
			return nil, fmt.Errorf("writing identity file: %w", err)
		}

	default:
		return nil, fmt.Errorf("getting private key: %w", err)
	}

	return priv, nil
}

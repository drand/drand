package lp2p

import (
	"context"
	"fmt"
	"time"

	"github.com/drand/drand/client"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"

	bds "github.com/ipfs/go-ds-badger2"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/proto"
)

// GossipRelayConfig configures a gossip relay node.
type GossipRelayConfig struct {
	// ChainHash is a hash that uniquely identifies the drand chain.
	ChainHash    string
	PeerWith     []string
	Addr         string
	DataDir      string
	IdentityPath string
	CertPath     string
	Insecure     bool
	Client       client.Client
}

// GossipRelayNode is a gossip relay runtime.
type GossipRelayNode struct {
	l         log.Logger
	bootstrap []ma.Multiaddr
	ds        *bds.Datastore
	priv      crypto.PrivKey
	h         host.Host
	ps        *pubsub.PubSub
	t         *pubsub.Topic
	addrs     []ma.Multiaddr
	done      chan struct{}
}

// NewGossipRelayNode starts a new gossip relay node.
func NewGossipRelayNode(l log.Logger, cfg *GossipRelayConfig) (*GossipRelayNode, error) {
	bootstrap, err := ParseMultiaddrSlice(cfg.PeerWith)
	if err != nil {
		return nil, xerrors.Errorf("parsing peer-with: %w", err)
	}

	ds, err := bds.NewDatastore(cfg.DataDir, nil)
	if err != nil {
		return nil, xerrors.Errorf("opening datastore: %w", err)
	}

	priv, err := LoadOrCreatePrivKey(cfg.IdentityPath, l)
	if err != nil {
		return nil, xerrors.Errorf("loading p2p key: %w", err)
	}

	h, ps, err := ConstructHost(ds, priv, cfg.Addr, bootstrap, l)
	if err != nil {
		return nil, xerrors.Errorf("constructing host: %w", err)
	}

	addrs, err := h.Network().InterfaceListenAddresses()
	if err != nil {
		return nil, xerrors.Errorf("getting InterfaceListenAddresses: %w", err)
	}

	for _, a := range addrs {
		l.Info("relay_node", "has addr", "addr", fmt.Sprintf("%s/p2p/%s", a, h.ID()))
	}

	t, err := ps.Join(PubSubTopic(cfg.ChainHash))
	if err != nil {
		return nil, xerrors.Errorf("joining topic: %w", err)
	}

	g := &GossipRelayNode{
		l:         l,
		bootstrap: bootstrap,
		ds:        ds,
		priv:      priv,
		h:         h,
		ps:        ps,
		t:         t,
		addrs:     addrs,
		done:      make(chan struct{}),
	}

	if cfg.Client == nil {
		return nil, xerrors.Errorf("No client supplying randomness supplied.")
	}
	go g.background(cfg.Client)

	return g, nil
}

// Multiaddrs returns the gossipsub multiaddresses of this relay node.
func (g *GossipRelayNode) Multiaddrs() []ma.Multiaddr {
	base := g.h.Addrs()
	b := make([]ma.Multiaddr, len(base))
	for i, a := range base {
		m, err := ma.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", a, g.h.ID()))
		if err != nil {
			panic(err)
		}
		b[i] = m
	}
	return b
}

// Shutdown stops the relay node.
func (g *GossipRelayNode) Shutdown() {
	close(g.done)
}

// ParseMultiaddrSlice parses a list of addresses into multiaddrs
func ParseMultiaddrSlice(peers []string) ([]ma.Multiaddr, error) {
	out := make([]ma.Multiaddr, len(peers))
	for i, peer := range peers {
		m, err := ma.NewMultiaddr(peer)
		if err != nil {
			return nil, xerrors.Errorf("parsing multiaddr\"%s\": %w", peer, err)
		}
		out[i] = m
	}
	return out, nil
}

func (g *GossipRelayNode) background(w client.Watcher) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		results := w.Watch(ctx)
	LOOP:
		for {
			select {
			case res, ok := <-results:
				if !ok {
					g.l.Warn("relay_node", "watch channel closed")
					break LOOP
				}

				rd, ok := res.(*client.RandomData)
				if !ok {
					g.l.Error("relay_node", "unexpected client result type")
					continue
				}

				randB, err := proto.Marshal(&drand.PublicRandResponse{
					Round:             res.Round(),
					Signature:         res.Signature(),
					PreviousSignature: rd.PreviousSignature,
					Randomness:        res.Randomness(),
				})
				if err != nil {
					g.l.Error("relay_node", "err marshaling", "err", err)
					continue
				}

				err = g.t.Publish(ctx, randB)
				if err != nil {
					g.l.Error("relay_node", "err publishing on pubsub", "err", err)
					continue
				}

				g.l.Info("relay_node", "Published randomness on pubsub", "round", res.Round())
			case <-g.done:
				return
			}
		}
		time.Sleep(time.Second)
	}
}

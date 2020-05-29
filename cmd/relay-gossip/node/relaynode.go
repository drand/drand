package node

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/drand/drand/client"
	dclient "github.com/drand/drand/client"
	"github.com/drand/drand/cmd/relay-gossip/lp2p"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/gogo/protobuf/proto"
	bds "github.com/ipfs/go-ds-badger2"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// GossipRelayConfig configures a gossip relay node.
type GossipRelayConfig struct {
	// ChainHash is a hash that uniquely identifies the drand chain.
	ChainHash       string
	PeerWith        []string
	Addr            string
	DataDir         string
	IdentityPath    string
	CertPath        string
	Insecure        bool
	DrandPublicGRPC string
	// DrandPublicHTTP are drand public HTTP API URLs to relay
	DrandPublicHTTP []string
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
	opts      []grpc.DialOption
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

	priv, err := lp2p.LoadOrCreatePrivKey(cfg.IdentityPath)
	if err != nil {
		return nil, xerrors.Errorf("loading p2p key: %w", err)
	}

	h, ps, err := lp2p.ConstructHost(ds, priv, cfg.Addr, bootstrap)
	if err != nil {
		return nil, xerrors.Errorf("constructing host: %w", err)
	}

	addrs, err := h.Network().InterfaceListenAddresses()
	if err != nil {
		return nil, xerrors.Errorf("getting InterfaceListenAddresses: %w", err)
	}

	for _, a := range addrs {
		l.Info(fmt.Sprintf("%s/p2p/%s\n", a, h.ID()))
	}

	t, err := ps.Join(lp2p.PubSubTopic(cfg.ChainHash))
	if err != nil {
		return nil, xerrors.Errorf("joining topic: %w", err)
	}

	opts := []grpc.DialOption{}
	if cfg.DrandPublicGRPC != "" {
		if cfg.CertPath != "" {
			creds, err := credentials.NewClientTLSFromFile(cfg.CertPath, "")
			if err != nil {
				return nil, xerrors.Errorf("loading cert file: %w", err)
			}
			opts = append(opts, grpc.WithTransportCredentials(creds))
		} else if cfg.Insecure {
			opts = append(opts, grpc.WithInsecure())
		} else {
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
		}
	}
	g := &GossipRelayNode{
		l:         l,
		bootstrap: bootstrap,
		ds:        ds,
		priv:      priv,
		h:         h,
		ps:        ps,
		t:         t,
		opts:      opts,
		addrs:     addrs,
		done:      make(chan struct{}),
	}

	if cfg.DrandPublicGRPC != "" {
		go g.startGRPC(cfg.DrandPublicGRPC)
	} else if len(cfg.DrandPublicHTTP) > 0 {
		go g.startHTTP(cfg.DrandPublicHTTP)
	} else {
		return nil, errors.New("missing gRPC or HTTP API addresses")
	}

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

func (g *GossipRelayNode) startGRPC(drandPublicGRPC string) {
	for {
		select {
		case <-g.done:
			return
		default:
		}
		conn, err := grpc.Dial(drandPublicGRPC, g.opts...)
		if err != nil {
			g.l.Warn(fmt.Sprintf("error connecting to grpc: %+v", err))
			time.Sleep(5 * time.Second)
			continue
		}
		client := drand.NewPublicClient(conn)
		err = g.workRelay(client)
		if err != nil {
			g.l.Warn(fmt.Sprintf("error relaying: %+v", err))
			err = conn.Close()
			if err != nil {
				g.l.Warn(fmt.Sprintf("error while closing connection: %+v", err))
			}
			time.Sleep(5 * time.Second)
		}
	}
}

func (g *GossipRelayNode) startHTTP(urls []string) {
	c, err := dclient.New(dclient.WithInsecureHTTPEndpoints(urls))
	if err != nil {
		g.l.Error("relaynode", "creating drand HTTP client", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := c.Watch(ctx)

	for {
		select {
		case res, ok := <-ch:
			if !ok {
				return
			}

			rd, ok := res.(*client.RandomData)
			if !ok {
				g.l.Error("relaynode", "unexpected client result type")
				continue
			}

			randB, err := proto.Marshal(&drand.PublicRandResponse{
				Round:             res.Round(),
				Signature:         res.Signature(),
				PreviousSignature: rd.PreviousSignature,
				Randomness:        res.Randomness(),
			})
			if err != nil {
				g.l.Error("relaynode", "marshaling", err)
				continue
			}

			err = g.t.Publish(ctx, randB)
			if err != nil {
				g.l.Error("relaynode", "publishing on pubsub", err)
				continue
			}

			g.l.Info("relaynode", "Published randomness on pubsub", "round", res.Round())
		case <-g.done:
			return
		}
	}
}

func (g *GossipRelayNode) workRelay(client drand.PublicClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	curr, err := client.PublicRand(ctx, &drand.PublicRandRequest{Round: 0})
	if err != nil {
		return xerrors.Errorf("getting initial round failed: %w", err)
	}
	g.l.Info(fmt.Sprintf("got latest rand: %d", curr.Round))

	// context.Background() on purpose as this applies to whole, long lived stream
	stream, err := client.PublicRandStream(context.Background(), &drand.PublicRandRequest{Round: curr.Round})
	if err != nil {
		return xerrors.Errorf("getting rand stream: %w", err)
	}

	for {
		select {
		case <-g.done:
			return xerrors.Errorf("relay shutdown")
		default:
		}
		rand, err := stream.Recv()
		if err != nil {
			return xerrors.Errorf("receving on stream: %w", err)
		}

		randB, err := proto.Marshal(rand)
		if err != nil {
			return xerrors.Errorf("marshaling: %w", err)
		}

		err = g.t.Publish(context.TODO(), randB)
		if err != nil {
			return xerrors.Errorf("publishing on pubsub: %w", err)
		}
		g.l.Info(fmt.Sprintf("Published randomness on pubsub, round: %d", rand.Round))
	}
}

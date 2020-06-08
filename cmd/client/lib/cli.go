package lib

import (
	"context"
	"encoding/hex"
	nhttp "net/http"
	"os"
	"path"
	"strconv"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/grpc"
	"github.com/drand/drand/client/http"
	"github.com/drand/drand/log"
	"github.com/drand/drand/lp2p"
	gclient "github.com/drand/drand/lp2p/client"

	"github.com/google/uuid"
	bds "github.com/ipfs/go-ds-badger2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
)

var (
	// ClientFlags is a list of common flags for client creation
	ClientFlags = []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "url",
			Usage: "root URL(s) for fetching randomness",
		},
		&cli.StringFlag{
			Name:    "grpc-connect",
			Usage:   "host:port to dial a gRPC randomness provider",
			Aliases: []string{"connect"},
		},
		&cli.StringFlag{
			Name:  "cert",
			Usage: "Path to a file containing gRPC transport credentials of peer",
		},
		&cli.StringFlag{
			Name:  "hash",
			Usage: "The hash (in hex) for the chain to follow",
		},
		&cli.BoolFlag{
			Name:  "insecure",
			Usage: "Allow autodetection of the chain information",
		},
		&cli.StringSliceFlag{
			Name:  "relay",
			Usage: "relay peer multiaddr(s) to connect with",
		},
		&cli.IntFlag{
			Name:  "port",
			Usage: "Local port for client to bind to, when connecting to relays",
		},
	}
)

// Create builds a client, and can be invoked from a cli action supplied
// with ClientFlags
func Create(c *cli.Context, withInstrumentation bool, opts ...client.Option) (client.Client, error) {
	if c.IsSet("grpc-connect") {
		return grpc.New(c.String("grpc-connect"), c.String("cert"), c.IsSet("insecure"))
	}
	var hash []byte
	var err error
	if c.IsSet("hash") {
		hash, err = hex.DecodeString(c.String("hash"))
		if err != nil {
			return nil, err
		}
		opts = append(opts, client.WithChainHash(hash))
	}
	if c.IsSet("insecure") {
		opts = append(opts, client.Insecurely())
	}
	httpClients := make([]client.Client, 0)
	var info *chain.Info
	for _, url := range c.StringSlice("url") {
		var hc client.Client
		if info != nil {
			hc, err = http.NewWithInfo(url, info, nhttp.DefaultTransport)
			if err != nil {
				log.DefaultLogger.Warn("client", "failed to load URL", "url", url, "err", err)
				continue
			}
		} else {
			hc, err = http.New(url, hash, nhttp.DefaultTransport)
			if err != nil {
				log.DefaultLogger.Warn("client", "failed to load URL", "url", url, "err", err)
				continue
			}
			info, err = hc.Info(context.Background())
			if err != nil {
				log.DefaultLogger.Warn("client", "failed to load Info from URL", "url", url, "err", err)
				continue
			}
		}
		httpClients = append(httpClients, hc)
	}
	if withInstrumentation {
		http.MeasureHeartbeats(c.Context, httpClients)
	}
	opts = append(opts, client.From(httpClients...))

	if c.IsSet("relays") {
		relayPeers, err := lp2p.ParseMultiaddrSlice(c.StringSlice("relays"))
		if err != nil {
			return nil, err
		}
		ps, err := buildClientHost(c.Int("port"), relayPeers)
		if err != nil {
			return nil, err
		}
		opts = append(opts, gclient.WithPubsub(ps))
	}

	return client.New(opts...)
}

func buildClientHost(clientRelayPort int, relayMultiaddr []ma.Multiaddr) (*pubsub.PubSub, error) {
	clientID := uuid.New().String()
	ds, err := bds.NewDatastore(path.Join(os.TempDir(), "drand-"+clientID+"-datastore"), nil)
	if err != nil {
		return nil, err
	}
	priv, err := lp2p.LoadOrCreatePrivKey(path.Join(os.TempDir(), "drand-"+clientID+"-id"), log.DefaultLogger)
	if err != nil {
		return nil, err
	}
	_, ps, err := lp2p.ConstructHost(
		ds,
		priv,
		"/ip4/0.0.0.0/tcp/"+strconv.Itoa(clientRelayPort),
		relayMultiaddr,
		log.DefaultLogger,
	)
	if err != nil {
		return nil, err
	}
	return ps, nil
}

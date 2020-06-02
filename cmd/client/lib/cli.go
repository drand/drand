package lib

import (
	"encoding/hex"
	"os"
	"path"
	"strconv"

	"github.com/drand/drand/client"
	"github.com/drand/drand/client/grpc"
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
			Usage: "root URLs for fetching randomness",
		},
		&cli.StringFlag{
			Name:  "grpc-connect",
			Usage: "host:port to dial a GRPC randomness provider",
		},
		&cli.StringFlag{
			Name:  "cert",
			Usage: "file containing GRPC transport credentials of peer",
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
			Name:  "relays",
			Usage: "list of multiaddresses of relays to connect with",
		},
		&cli.IntFlag{
			Name:  "port",
			Usage: "Local port for client to bind to, when connecting to relays",
		},
	}
)

// Create builds a client, and can be invoked from a cli action supplied
// with ClientFlags
func Create(c *cli.Context, opts ...client.Option) (client.Client, error) {
	if c.IsSet("grpc-connect") {
		return grpc.New(c.String("grpc-connect"), c.String("cert"), c.IsSet("insecure"))
	}
	if c.IsSet("hash") {
		hex, err := hex.DecodeString(c.String("hash"))
		if err != nil {
			return nil, err
		}
		opts = append(opts, client.WithChainHash(hex))
	}
	if c.IsSet("insecure") {
		opts = append(opts, client.WithInsecureHTTPEndpoints(c.StringSlice("url")))
	} else {
		opts = append(opts, client.WithHTTPEndpoints(c.StringSlice("url")))
	}

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

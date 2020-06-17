package lib

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	nhttp "net/http"
	"os"
	"path"
	"strings"
	"time"

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
	// URLFlag is the CLI flag for root URL(s) for fetching randomness.
	URLFlag = &cli.StringSliceFlag{
		Name:    "url",
		Usage:   "root URL(s) for fetching randomness",
		Aliases: []string{"http-failover"}, // DEPRECATED
	}
	// GRPCConnectFlag is the CLI flag for host:port to dial a gRPC randomness
	// provider.
	GRPCConnectFlag = &cli.StringFlag{
		Name:    "grpc-connect",
		Usage:   "host:port to dial a gRPC randomness provider",
		Aliases: []string{"connect"}, // DEPRECATED
	}
	// CertFlag is the CLI flag for the path to a file containing gRPC transport
	// credentials of peer.
	CertFlag = &cli.StringFlag{
		Name:  "cert",
		Usage: "Path to a file containing gRPC transport credentials of peer",
	}
	// HashFlag is the CLI flag for the hash (in hex) for the chain to follow.
	HashFlag = &cli.StringFlag{
		Name:    "hash",
		Usage:   "The hash (in hex) for the chain to follow",
		Aliases: []string{"chain-hash"}, // DEPRECATED
	}
	// InsecureFlag is the CLI flag to allow autodetection of the chain
	// information.
	InsecureFlag = &cli.BoolFlag{
		Name:  "insecure",
		Usage: "Allow autodetection of the chain information",
	}
	// RelayFlag is the CLI flag for relay peer multiaddr(s) to connect with.
	RelayFlag = &cli.StringSliceFlag{
		Name:    "relay",
		Usage:   "relay peer multiaddr(s) to connect with",
		Aliases: []string{"peer-with"}, // DEPRECATED
	}
	// PortFlag is the CLI flag for local address for client to bind to, when
	// connecting to relays. (specified as a numeric port, or a host:port)
	PortFlag = &cli.StringFlag{
		Name:  "port",
		Usage: "Local (host:)port for client to bind to, when connecting to relays",
	}
	// FailoverGraceFlag is the CLI flag for setting the grace period before
	// randomness is requested from the HTTP API when watching for randomness
	// and it does not arrive.
	FailoverGraceFlag = &cli.DurationFlag{
		Name:    "failover-grace",
		Usage:   "grace period before randomness is requested from the HTTP API when watching for randomness and it does not arrive (default 5s)",
		Aliases: []string{"http-failover-grace"}, // DEPRECATED
	}
)

// ClientFlags is a list of common flags for client creation
var ClientFlags = []cli.Flag{
	URLFlag,
	GRPCConnectFlag,
	CertFlag,
	HashFlag,
	InsecureFlag,
	RelayFlag,
	PortFlag,
	FailoverGraceFlag,
}

// Create builds a client, and can be invoked from a cli action supplied
// with ClientFlags
func Create(c *cli.Context, withInstrumentation bool, opts ...client.Option) (client.Client, error) {
	clients := make([]client.Client, 0)
	var info *chain.Info

	if c.IsSet(GRPCConnectFlag.Name) {
		gc, err := grpc.New(c.String(GRPCConnectFlag.Name), c.String(CertFlag.Name), c.Bool(InsecureFlag.Name))
		if err != nil {
			return nil, err
		}
		info, err = gc.Info(context.Background())
		if err != nil {
			return nil, err
		}
		clients = append(clients, gc)
	}

	var hash []byte
	var err error
	if c.IsSet(HashFlag.Name) {
		hash, err = hex.DecodeString(c.String(HashFlag.Name))
		if err != nil {
			return nil, err
		}
		opts = append(opts, client.WithChainHash(hash))
	}
	if c.Bool(InsecureFlag.Name) {
		opts = append(opts, client.Insecurely())
	}
	skipped := []string{}
	var hc client.Client
	for _, url := range c.StringSlice(URLFlag.Name) {
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
				skipped = append(skipped, url)
				continue
			}
			info, err = hc.Info(context.Background())
			if err != nil {
				log.DefaultLogger.Warn("client", "failed to load Info from URL", "url", url, "err", err)
				continue
			}
		}
		clients = append(clients, hc)
	}
	if info != nil {
		for _, url := range skipped {
			hc, err = http.NewWithInfo(url, info, nhttp.DefaultTransport)
			if err != nil {
				log.DefaultLogger.Warn("client", "failed to load URL", "url", url, "err", err)
				continue
			}
			clients = append(clients, hc)
		}
	}
	if withInstrumentation {
		http.MeasureHeartbeats(c.Context, clients)
	}

	if c.IsSet(RelayFlag.Name) {
		addrs := c.StringSlice(RelayFlag.Name)
		if len(addrs) > 0 {
			relayPeers, err := lp2p.ParseMultiaddrSlice(addrs)
			if err != nil {
				return nil, err
			}
			listen := ""
			if c.IsSet(PortFlag.Name) {
				listen = c.String(PortFlag.Name)
			}
			ps, err := buildClientHost(listen, relayPeers)
			if err != nil {
				return nil, err
			}
			opts = append(opts, gclient.WithPubsub(ps))
		}
	}

	if c.IsSet(FailoverGraceFlag.Name) {
		grace := c.Duration(FailoverGraceFlag.Name)
		if grace == 0 {
			grace = time.Second * 5
		}
		opts = append(opts, client.WithFailoverGracePeriod(grace))
	}

	return client.Wrap(clients, opts...)
}

func buildClientHost(clientListenAddr string, relayMultiaddr []ma.Multiaddr) (*pubsub.PubSub, error) {
	clientID := uuid.New().String()
	ds, err := bds.NewDatastore(path.Join(os.TempDir(), "drand-"+clientID+"-datastore"), nil)
	if err != nil {
		return nil, err
	}
	priv, err := lp2p.LoadOrCreatePrivKey(path.Join(os.TempDir(), "drand-"+clientID+"-id"), log.DefaultLogger)
	if err != nil {
		return nil, err
	}

	listen := ""
	if clientListenAddr != "" {
		bindHost := "0.0.0.0"
		if strings.Contains(clientListenAddr, ":") {
			host, port, err := net.SplitHostPort(clientListenAddr)
			if err != nil {
				return nil, err
			}
			bindHost = host
			clientListenAddr = port
		}
		listen = fmt.Sprintf("/ip4/%s/tcp/%s", bindHost, clientListenAddr)
	}

	_, ps, err := lp2p.ConstructHost(
		ds,
		priv,
		listen,
		relayMultiaddr,
		log.DefaultLogger,
	)
	if err != nil {
		return nil, err
	}
	return ps, nil
}

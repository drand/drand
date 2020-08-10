package lib

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net"
	nhttp "net/http"
	"os"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/grpc"
	"github.com/drand/drand/client/http"
	"github.com/drand/drand/key"
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
	// GroupConfFlag is the CLI flag for specifying the path to the drand group configuration (TOML encoded) or chain info (JSON encoded).
	GroupConfFlag = &cli.PathFlag{
		Name: "group-conf",
		Usage: "Path to a drand group configuration (TOML encoded) or chain info (JSON encoded)," +
			" can be used instead of `-hash` flag to verify the chain.",
	}
	// InsecureFlag is the CLI flag to allow autodetection of the chain
	// information.
	InsecureFlag = &cli.BoolFlag{
		Name:  "insecure",
		Usage: "Allow autodetection of the chain information",
	}
	// RelayFlag is the CLI flag for relay peer multiaddr(s) to connect with.
	RelayFlag = &cli.StringSliceFlag{
		Name:  "relay",
		Usage: "relay peer multiaddr(s) to connect with",
	}
	// PortFlag is the CLI flag for local address for client to bind to, when
	// connecting to relays. (specified as a numeric port, or a host:port)
	PortFlag = &cli.StringFlag{
		Name:  "port",
		Usage: "Local (host:)port for constructed libp2p host to listen on",
	}
)

// ClientFlags is a list of common flags for client creation
var ClientFlags = []cli.Flag{
	URLFlag,
	GRPCConnectFlag,
	CertFlag,
	HashFlag,
	GroupConfFlag,
	InsecureFlag,
	RelayFlag,
	PortFlag,
}

// Create builds a client, and can be invoked from a cli action supplied
// with ClientFlags
func Create(c *cli.Context, withInstrumentation bool, opts ...client.Option) (client.Client, error) {
	clients := make([]client.Client, 0)
	var info *chain.Info
	var err error

	if c.IsSet(GroupConfFlag.Name) {
		info, err = chainInfoFromGroupTOML(c.Path(GroupConfFlag.Name))
		if err != nil {
			info, _ = chainInfoFromChainInfoJSON(c.Path(GroupConfFlag.Name))
			if info == nil {
				return nil, fmt.Errorf("failed to decode group configuration: %w", err)
			}
		}
		opts = append(opts, client.WithChainInfo(info))
	}

	gc, err := buildGrpcClient(c, &info)
	if err != nil {
		return nil, err
	}
	clients = append(clients, gc...)

	var hash []byte
	if c.IsSet(HashFlag.Name) {
		hash, err = hex.DecodeString(c.String(HashFlag.Name))
		if err != nil {
			return nil, err
		}
		if info != nil && !bytes.Equal(hash, info.Hash()) {
			return nil, fmt.Errorf(
				"incorrect chain hash %v != %v",
				c.String(HashFlag.Name),
				hex.EncodeToString(info.Hash()),
			)
		}
		opts = append(opts, client.WithChainHash(hash))
	}
	if c.Bool(InsecureFlag.Name) {
		opts = append(opts, client.Insecurely())
	}

	clients = append(clients, buildHTTPClients(c, &info, hash, withInstrumentation)...)

	gopt, err := buildGossipClient(c)
	if err != nil {
		return nil, err
	}
	opts = append(opts, gopt...)

	return client.Wrap(clients, opts...)
}

func buildGrpcClient(c *cli.Context, info **chain.Info) ([]client.Client, error) {
	if c.IsSet(GRPCConnectFlag.Name) {
		gc, err := grpc.New(c.String(GRPCConnectFlag.Name), c.String(CertFlag.Name), c.Bool(InsecureFlag.Name))
		if err != nil {
			return nil, err
		}
		if *info == nil {
			*info, err = gc.Info(context.Background())
			if err != nil {
				return nil, err
			}
		}
		return []client.Client{gc}, nil
	}
	return []client.Client{}, nil
}

func buildHTTPClients(c *cli.Context, info **chain.Info, hash []byte, withInstrumentation bool) []client.Client {
	clients := make([]client.Client, 0)
	var err error
	skipped := []string{}
	var hc client.Client
	for _, url := range c.StringSlice(URLFlag.Name) {
		if *info != nil {
			hc, err = http.NewWithInfo(url, *info, nhttp.DefaultTransport)
			if err != nil {
				log.DefaultLogger().Warn("client", "failed to load URL", "url", url, "err", err)
				continue
			}
		} else {
			hc, err = http.New(url, hash, nhttp.DefaultTransport)
			if err != nil {
				log.DefaultLogger().Warn("client", "failed to load URL", "url", url, "err", err)
				skipped = append(skipped, url)
				continue
			}
			*info, err = hc.Info(context.Background())
			if err != nil {
				log.DefaultLogger().Warn("client", "failed to load Info from URL", "url", url, "err", err)
				continue
			}
		}
		clients = append(clients, hc)
	}
	if *info != nil {
		for _, url := range skipped {
			hc, err = http.NewWithInfo(url, *info, nhttp.DefaultTransport)
			if err != nil {
				log.DefaultLogger().Warn("client", "failed to load URL", "url", url, "err", err)
				continue
			}
			clients = append(clients, hc)
		}
	}

	if withInstrumentation {
		http.MeasureHeartbeats(c.Context, clients)
	}

	return clients
}

func buildGossipClient(c *cli.Context) ([]client.Option, error) {
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
			return []client.Option{gclient.WithPubsub(ps)}, nil
		}
	}
	return []client.Option{}, nil
}

func buildClientHost(clientListenAddr string, relayMultiaddr []ma.Multiaddr) (*pubsub.PubSub, error) {
	clientID := uuid.New().String()
	ds, err := bds.NewDatastore(path.Join(os.TempDir(), "drand-"+clientID+"-datastore"), nil)
	if err != nil {
		return nil, err
	}
	priv, err := lp2p.LoadOrCreatePrivKey(path.Join(os.TempDir(), "drand-"+clientID+"-id"), log.DefaultLogger())
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
		log.DefaultLogger(),
	)
	if err != nil {
		return nil, err
	}
	return ps, nil
}

// chainInfoFromGroupTOML reads a drand group TOML file and returns the chain info.
func chainInfoFromGroupTOML(filePath string) (*chain.Info, error) {
	gt := &key.GroupTOML{}
	_, err := toml.DecodeFile(filePath, gt)
	if err != nil {
		return nil, err
	}
	g := &key.Group{}
	err = g.FromTOML(gt)
	if err != nil {
		return nil, err
	}
	return chain.NewChainInfo(g), nil
}

func chainInfoFromChainInfoJSON(filePath string) (*chain.Info, error) {
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return chain.InfoFromJSON(bytes.NewBuffer(b))
}

package lib

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"net"
	nhttp "net/http"
	"os"
	"path"
	"strings"

	pubClient "github.com/drand/drand/client"
	"github.com/drand/drand/client/grpc"
	http2 "github.com/drand/drand/client/http"
	gclient "github.com/drand/drand/client/lp2p"
	commonutils "github.com/drand/drand/common"
	chainCommon "github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/client"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/lp2p"
	"github.com/google/uuid"
	bds "github.com/ipfs/go-ds-badger2"
	clock "github.com/jonboulle/clockwork"
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
	// HashFlag is the CLI flag for the hash (in hex) of the targeted chain.
	HashFlag = &cli.StringFlag{
		Name:    "hash",
		Usage:   "The hash (in hex) of the chain to follow",
		Aliases: []string{"chain-hash"},
	}
	// HashListFlag is the CLI flag for the hashes list (in hex) for the relay to follow.
	HashListFlag = &cli.StringSliceFlag{
		Name:  "hash-list",
		Usage: "Specify the list (in hex) of hashes the relay should follow",
	}
	// GroupConfFlag is the CLI flag for specifying the path to the drand group configuration (TOML encoded) or chain info (JSON encoded).
	GroupConfFlag = &cli.PathFlag{
		Name: "group-conf",
		Usage: "Path to a drand group configuration (TOML encoded) or chain info (JSON encoded)," +
			" can be used instead of `-hash` flag to verify the chain.",
	}
	// GroupConfListFlag is like GroupConfFlag but for a list values.
	GroupConfListFlag = &cli.StringSliceFlag{
		Name: "group-conf-list",
		Usage: "Paths to at least one drand group configuration (TOML encoded) or chain info (JSON encoded)," +
			fmt.Sprintf(" can be used instead of `-%s` flag to verify the chain.", HashListFlag.Name),
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

	// JSONFlag is the value of the CLI flag `json` enabling JSON output of the loggers
	JSONFlag = &cli.BoolFlag{
		Name:  "json",
		Usage: "Set the output as json format",
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
	JSONFlag,
}

// Create builds a client, and can be invoked from a cli action supplied
// with ClientFlags
func Create(c *cli.Context, withInstrumentation bool, opts ...pubClient.Option) (client.Client, error) {
	ctx := c.Context
	l := log.FromContextOrDefault(ctx)
	clients := make([]client.Client, 0)
	var info *chainCommon.Info
	var err error

	if groupPath := c.Path(GroupConfFlag.Name); groupPath != "" {
		info, err = chainCommon.InfoFromGroupTOML(l, groupPath)
		if err != nil {
			info, err = chainCommon.InfoFromChainInfoJSON(groupPath)
			if info == nil || err != nil {
				return nil, fmt.Errorf("failed to decode group (%s) : %w", groupPath, err)
			}
		}
		opts = append(opts, pubClient.WithChainInfo(info))
	}

	gc, info, err := buildGrpcClient(c, l, info)
	if err != nil {
		return nil, err
	}
	if len(gc) > 0 {
		clients = append(clients, gc...)
	}

	var hash []byte
	if c.IsSet(HashFlag.Name) && c.String(HashFlag.Name) != "" {
		hash, err = hex.DecodeString(c.String(HashFlag.Name))
		if err != nil {
			return nil, err
		}
		if info != nil && !bytes.Equal(hash, info.Hash()) {
			return nil, fmt.Errorf(
				"%w for beacon %s %v != %v",
				commonutils.ErrInvalidChainHash,
				info.ID,
				c.String(HashFlag.Name),
				hex.EncodeToString(info.Hash()),
			)
		}
		opts = append(opts, pubClient.WithChainHash(hash))
	}

	if c.Bool(InsecureFlag.Name) {
		opts = append(opts, pubClient.Insecurely())
	}

	clients = append(clients, buildHTTPClients(c, l, &info, hash, withInstrumentation)...)

	gopt, err := buildGossipClient(c, l)
	if err != nil {
		return nil, err
	}
	opts = append(opts, gopt...)

	return pubClient.Wrap(ctx, l, clients, opts...)
}

func buildGrpcClient(c *cli.Context, l log.Logger, info *chainCommon.Info) ([]client.Client, *chainCommon.Info, error) {
	if !c.IsSet(GRPCConnectFlag.Name) {
		return nil, info, nil
	}

	var hash []byte
	if c.IsSet(HashFlag.Name) {
		var err error

		hash, err = hex.DecodeString(c.String(HashFlag.Name))
		if err != nil {
			return nil, nil, err
		}
	}

	if info != nil && len(hash) == 0 {
		hash = info.Hash()
	}

	gc, err := grpc.New(l, c.String(GRPCConnectFlag.Name), c.String(CertFlag.Name), c.Bool(InsecureFlag.Name), hash)
	if err != nil {
		return nil, nil, err
	}

	if info == nil {
		info, err = gc.Info(c.Context)
		if err != nil {
			return nil, nil, err
		}
	}

	return []client.Client{gc}, info, nil
}

func buildHTTPClients(c *cli.Context, l log.Logger, info **chainCommon.Info, hash []byte, withInstrumentation bool) []client.Client {
	ctx := c.Context
	clients := make([]client.Client, 0)
	var err error
	var skipped []string
	var hc client.Client
	for _, url := range c.StringSlice(URLFlag.Name) {
		if *info != nil {
			hc, err = http2.NewWithInfo(l, url, *info, nhttp.DefaultTransport)
			if err != nil {
				l.Warnw("", "client", "failed to load URL", "url", url, "err", err)
				continue
			}
		} else {
			hc, err = http2.New(ctx, l, url, hash, nhttp.DefaultTransport)
			if err != nil {
				l.Warnw("", "client", "failed to load URL", "url", url, "err", err)
				skipped = append(skipped, url)
				continue
			}
			*info, err = hc.Info(ctx)
			if err != nil {
				l.Warnw("", "client", "failed to load Info from URL", "url", url, "err", err)
				continue
			}
		}
		clients = append(clients, hc)
	}
	if *info != nil {
		for _, url := range skipped {
			hc, err = http2.NewWithInfo(l, url, *info, nhttp.DefaultTransport)
			if err != nil {
				l.Warnw("", "client", "failed to load URL", "url", url, "err", err)
				continue
			}
			clients = append(clients, hc)
		}
	}

	if withInstrumentation {
		http2.MeasureHeartbeats(c.Context, clients)
	}

	return clients
}

func buildGossipClient(c *cli.Context, l log.Logger) ([]pubClient.Option, error) {
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
			ps, err := buildClientHost(l, listen, relayPeers)
			if err != nil {
				return nil, err
			}
			return []pubClient.Option{gclient.WithPubsub(l, ps, clock.NewRealClock(), gclient.DefaultBufferSize)}, nil
		}
	}
	return []pubClient.Option{}, nil
}

func buildClientHost(l log.Logger, clientListenAddr string, relayMultiaddr []ma.Multiaddr) (*pubsub.PubSub, error) {
	clientID := uuid.New().String()
	ds, err := bds.NewDatastore(path.Join(os.TempDir(), "drand-"+clientID+"-datastore"), nil)
	if err != nil {
		return nil, err
	}
	priv, err := lp2p.LoadOrCreatePrivKey(path.Join(os.TempDir(), "drand-"+clientID+"-id"), l)
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
		l,
	)
	if err != nil {
		return nil, err
	}
	return ps, nil
}

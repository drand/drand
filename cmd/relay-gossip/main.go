package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/grpc"
	dlog "github.com/drand/drand/log"
	"github.com/drand/drand/lp2p"
	psc "github.com/drand/drand/lp2p/client"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/metrics/pprof"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/ipfs/go-datastore"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	peer "github.com/libp2p/go-libp2p-core/peer"
	cli "github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	version   = "master"
	gitCommit = "none"
	buildDate = "unknown"
)

var log = dlog.DefaultLogger

func main() {
	app := &cli.App{
		Name:    "drand-relay-gossip",
		Version: version,
		Usage:   "pubsub relay for randomness beacon",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "chain-hash",
			},
		},
		Commands: []*cli.Command{runCmd, clientCmd, idCmd},
	}
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("drand gossip relay %v (date %v, commit %v)\n", version, buildDate, gitCommit)
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("error: %+v\n", err)
		os.Exit(1)
	}
}

var peerWithFlag = &cli.StringSliceFlag{
	Name:  "peer-with",
	Usage: "list of peers to connect with",
}

var httpFailoverFlag = &cli.StringSliceFlag{
	Name:  "http-failover",
	Usage: "URL(s) of drand HTTP API(s) to failover to if randomness rounds do not arrive in time",
}

var httpFailoverGraceFlag = &cli.DurationFlag{
	Name:  "http-failover-grace",
	Usage: "grace period before the failover HTTP API is used when watching for randomness (default 5s)",
}

var runCmd = &cli.Command{
	Name: "run",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "grpc-connect",
			Usage:   "host:port to dial to a drand gRPC API",
			Aliases: []string{"connect"},
		},
		&cli.StringSliceFlag{
			Name:  "http-connect",
			Usage: "URL(s) of drand HTTP API(s) to relay",
		},
		&cli.StringFlag{
			Name:  "store",
			Usage: "datastore directory",
			Value: "./datastore",
		},
		&cli.StringFlag{
			Name:  "cert",
			Usage: "file containing GRPC transport credentials of peer",
		},
		&cli.BoolFlag{
			Name:  "insecure",
			Usage: "allow insecure connection",
		},
		&cli.StringFlag{
			Name:  "listen",
			Usage: "listen addr for libp2p",
			Value: "/ip4/0.0.0.0/tcp/44544",
		},
		&cli.StringFlag{
			Name:  "metrics",
			Usage: "local host:port to bind a metrics servlet (optional)",
		},
		peerWithFlag,
		idFlag,
		httpFailoverFlag,
		httpFailoverGraceFlag,
	},

	Action: func(cctx *cli.Context) error {
		if cctx.IsSet("metrics") {
			metricsListener := metrics.Start(cctx.String("metrics"), pprof.WithProfile(), nil)
			defer metricsListener.Close()
			metrics.PrivateMetrics.Register(grpc_prometheus.DefaultClientMetrics)
		}

		var chainHash []byte
		var err error

		if cctx.String("chain-hash") != "" {
			chainHash, err = hex.DecodeString(cctx.String("chain-hash"))
			if err != nil {
				return xerrors.Errorf("decoding chain hash: %w", err)
			}
		}

		var c client.Client
		httpConnect := cctx.StringSlice("http-connect")

		if len(httpConnect) > 0 {
			opts := []client.Option{}
			if chainHash != nil {
				opts = append(opts, client.WithChainHash(chainHash), client.WithHTTPEndpoints(httpConnect))
			} else {
				opts = append(opts, client.WithInsecureHTTPEndpoints(httpConnect))
			}
			c, err = client.New(opts...)
		} else {
			grpcClient, err := grpc.New(cctx.String("grpc-connect"), cctx.String("cert"), cctx.Bool("insecure"))
			if err != nil {
				return xerrors.Errorf("constructing gRPC client: %w", err)
			}

			httpFailover := cctx.StringSlice(httpFailoverFlag.Name)
			if len(httpFailover) > 0 {
				chainInfo, err := c.Info(context.Background())
				if err != nil {
					return xerrors.Errorf("getting chain info: %w", err)
				}
				grace := cctx.Duration(httpFailoverGraceFlag.Name)
				if grace == 0 {
					grace = time.Second * 5
				}
				c, err = client.New(
					client.WithChainInfo(chainInfo),
					client.WithHTTPEndpoints(httpFailover),
					client.WithFailoverGracePeriod(grace),
					client.WithWatcher(func(_ *chain.Info, cache client.Cache) (client.Watcher, error) {
						return grpcClient, nil
					}),
				)
			} else {
				c = grpcClient
			}
		}
		if err != nil {
			return xerrors.Errorf("constructing client: %w", err)
		}

		cfg := &lp2p.GossipRelayConfig{
			ChainHash:    cctx.String("chain-hash"),
			PeerWith:     cctx.StringSlice(peerWithFlag.Name),
			Addr:         cctx.String("listen"),
			DataDir:      cctx.String("store"),
			IdentityPath: cctx.String(idFlag.Name),
			Client:       c,
		}
		if _, err := lp2p.NewGossipRelayNode(log, cfg); err != nil {
			return err
		}
		<-(chan int)(nil)
		return nil
	},
}

var clientCmd = &cli.Command{
	Name: "client",
	Flags: []cli.Flag{
		peerWithFlag,
		httpFailoverFlag,
		httpFailoverGraceFlag,
	},
	Action: func(cctx *cli.Context) error {
		bootstrap, err := lp2p.ParseMultiaddrSlice(cctx.StringSlice(peerWithFlag.Name))
		if err != nil {
			return xerrors.Errorf("parsing peer-with: %w", err)
		}

		priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return xerrors.Errorf("generating ed25519 key: %w", err)
		}

		_, ps, err := lp2p.ConstructHost(datastore.NewMapDatastore(), priv, "/ip4/0.0.0.0/tcp/0", bootstrap, log)
		if err != nil {
			return xerrors.Errorf("constructing host: %w", err)
		}

		chainHash, err := hex.DecodeString(cctx.String("chain-hash"))
		if err != nil {
			return xerrors.Errorf("decoding chain hash: %w", err)
		}

		httpFailover := cctx.StringSlice(httpFailoverFlag.Name)

		var c client.Watcher
		// if we have http failover endpoints then use the drand HTTP client with pubsub option
		if len(httpFailover) > 0 {
			grace := cctx.Duration(httpFailoverGraceFlag.Name)
			if grace == 0 {
				grace = time.Second * 5
			}
			c, err = client.New(
				psc.WithPubsub(ps),
				client.WithChainHash(chainHash),
				client.WithHTTPEndpoints(httpFailover),
				client.WithFailoverGracePeriod(grace),
			)
			if err != nil {
				return xerrors.Errorf("constructing client: %w", err)
			}
		} else {
			c, err = psc.NewWithPubsub(ps, nil, nil)
			if err != nil {
				return xerrors.Errorf("constructing client: %w", err)
			}
		}

		for rand := range c.Watch(context.Background()) {
			log.Info("client", "got randomness", "round", rand.Round(), "signature", rand.Signature()[:16])
		}

		return nil
	},
}

var idFlag = &cli.StringFlag{
	Name:  "identity",
	Usage: "path to a file containing libp2p identity",
	Value: "identity.key",
}

var idCmd = &cli.Command{
	Name:  "peerid",
	Usage: "prints libp2p peerid",

	Flags: []cli.Flag{idFlag},
	Action: func(cctx *cli.Context) error {
		priv, err := lp2p.LoadOrCreatePrivKey(cctx.String(idFlag.Name), log)
		if err != nil {
			return xerrors.Errorf("loading p2p key: %w", err)
		}
		peerID, err := peer.IDFromPrivateKey(priv)
		if err != nil {
			return xerrors.Errorf("computing peerid: %w", err)
		}
		fmt.Printf("%s\n", peerID)
		return nil
	},
}

package main

import (
	"crypto/rand"
	"fmt"
	"os"

	"github.com/drand/drand/cmd/relay-gossip/client"
	"github.com/drand/drand/cmd/relay-gossip/lp2p"
	"github.com/drand/drand/cmd/relay-gossip/node"
	dlog "github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/metrics/pprof"
	"github.com/drand/drand/protobuf/drand"
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
		Name:     "drand-relay-gossip",
		Version:  version,
		Usage:    "pubsub relay for drand randomness beacon",
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

var chainHashFlag = &cli.StringFlag{
	Name:     "chain-hash",
	Usage:    "hash of the drand group chain (hex encoded)",
	Aliases:  []string{"c"},
	Required: true,
}

var peerWithFlag = &cli.StringSliceFlag{
	Name:  "peer-with",
	Usage: "list of peer multiaddrs to direct connect with",
}

var idFlag = &cli.StringFlag{
	Name:  "identity",
	Usage: "path to a file containing a libp2p identity (base64 encoded)",
	Value: "identity.key",
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "starts a drand gossip relay process",
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
			Usage: "file containing gRPC transport credentials of peer",
		},
		&cli.BoolFlag{
			Name:  "insecure",
			Usage: "allow insecure gRPC connection",
		},
		&cli.StringFlag{
			Name:  "listen",
			Usage: "listening address for libp2p",
			Value: "/ip4/0.0.0.0/tcp/44544",
		},
		&cli.StringFlag{
			Name:  "metrics",
			Usage: "local host:port to bind a metrics servlet (optional)",
		},
		chainHashFlag,
		peerWithFlag,
		idFlag,
	},

	Action: func(cctx *cli.Context) error {
		if cctx.IsSet("metrics") {
			metricsListener := metrics.Start(cctx.String("metrics"), pprof.WithProfile(), nil)
			defer metricsListener.Close()
		}
		cfg := &node.GossipRelayConfig{
			ChainHash:       cctx.String("chain-hash"),
			PeerWith:        cctx.StringSlice(peerWithFlag.Name),
			Addr:            cctx.String("listen"),
			DataDir:         cctx.String("store"),
			IdentityPath:    cctx.String(idFlag.Name),
			CertPath:        cctx.String("cert"),
			Insecure:        cctx.Bool("insecure"),
			DrandPublicGRPC: cctx.String("grpc-connect"),
			DrandPublicHTTP: cctx.StringSlice("http-connect"),
		}
		if _, err := node.NewGossipRelayNode(log, cfg); err != nil {
			return err
		}
		<-(chan int)(nil)
		return nil
	},
}

var clientCmd = &cli.Command{
	Name:  "client",
	Usage: "starts a drand gossip client and prints out randomness as it is received",
	Flags: []cli.Flag{chainHashFlag, peerWithFlag},
	Action: func(cctx *cli.Context) error {
		bootstrap, err := node.ParseMultiaddrSlice(cctx.StringSlice(peerWithFlag.Name))
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

		c, err := client.NewWithPubsub(ps, nil, nil)
		if err != nil {
			return xerrors.Errorf("constructing client: %w", err)
		}

		var notifChan <-chan drand.PublicRandResponse
		var unsub client.UnsubFunc
		{
			ch := make(chan drand.PublicRandResponse, 5)
			notifChan = ch
			unsub = c.Sub(ch)
		}
		_ = unsub

		for rand := range notifChan {
			log.Info("client", "got randomness", "round", rand.Round, "signature", rand.Signature[:16])
		}
		return nil
	},
}

var idCmd = &cli.Command{
	Name:  "peerid",
	Usage: "prints the libp2p peer ID or creates one if it does not exist",
	Flags: []cli.Flag{idFlag},
	Action: func(cctx *cli.Context) error {
		priv, err := lp2p.LoadOrCreatePrivKey(cctx.String(idFlag.Name), log)
		if err != nil {
			return xerrors.Errorf("loading p2p key: %w", err)
		}
		peerId, err := peer.IDFromPrivateKey(priv)
		if err != nil {
			return xerrors.Errorf("computing peerid: %w", err)
		}
		fmt.Printf("%s\n", peerId)
		return nil
	},
}

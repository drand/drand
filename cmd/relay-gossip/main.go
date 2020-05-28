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
	logging "github.com/ipfs/go-log/v2"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	peer "github.com/libp2p/go-libp2p-core/peer"
	cli "github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var (
	log = logging.Logger("beacon-relay")
)

func main() {
	logging.SetLogLevel("*", "info")
	logging.SetLogLevel("beacon-relay", "info")

	app := &cli.App{
		Name:    "beacon-relay",
		Version: "0.0.1",
		Usage:   "pubsub relay for randomness beacon",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "chain-hash",
				Required: true,
			},
		},
		Commands: []*cli.Command{runCmd, clientCmd, idCmd},
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

var runCmd = &cli.Command{
	Name: "run",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "connect",
			Usage: "host:port to dial to a drand gRPC PI",
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
		peerWithFlag, idFlag,
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
			DrandPublicGRPC: cctx.String("connect"),
		}
		if _, err := node.NewGossipRelayNode(dlog.DefaultLogger, cfg); err != nil {
			return err
		}
		<-(chan int)(nil)
		return nil
	},
}

var clientCmd = &cli.Command{
	Name:  "client",
	Flags: []cli.Flag{peerWithFlag},
	Action: func(cctx *cli.Context) error {
		bootstrap, err := node.ParseMultiaddrSlice(cctx.StringSlice(peerWithFlag.Name))
		if err != nil {
			return xerrors.Errorf("parsing peer-with: %w", err)
		}

		priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return xerrors.Errorf("generating ed25519 key: %w", err)
		}

		_, ps, err := lp2p.ConstructHost(datastore.NewMapDatastore(), priv, "/ip4/0.0.0.0/tcp/0", bootstrap)
		if err != nil {
			return xerrors.Errorf("constructing host: %w", err)
		}

		c, err := client.NewWithPubsub(ps, cctx.String("chain-hash"))
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
			fmt.Printf("got randomness: Round %d: %X\n", rand.Round, rand.Signature[:16])
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
		priv, err := lp2p.LoadOrCreatePrivKey(cctx.String(idFlag.Name))
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

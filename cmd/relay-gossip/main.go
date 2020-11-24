package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/drand/drand/cmd/client/lib"
	"github.com/drand/drand/log"
	"github.com/drand/drand/lp2p"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/metrics/pprof"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	peer "github.com/libp2p/go-libp2p-core/peer"
	cli "github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.version=`git describe --tags`
//   -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	version   = "master"
	gitCommit = "none"
	buildDate = "unknown"
)

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

var (
	idFlag = &cli.StringFlag{
		Name:  "identity",
		Usage: "path to a file containing a libp2p identity (base64 encoded)",
		Value: "identity.key",
	}
	peerWithFlag = &cli.StringSliceFlag{
		Name:  "peer-with",
		Usage: "peer multiaddr(s) for the relay to direct connect with",
	}
	storeFlag = &cli.StringFlag{
		Name:  "store",
		Usage: "datastore directory",
		Value: "./datastore",
	}
	listenFlag = &cli.StringFlag{
		Name:  "listen",
		Usage: "listening address for libp2p",
		Value: "/ip4/0.0.0.0/tcp/44544",
	}
	metricsFlag = &cli.StringFlag{
		Name:  "metrics",
		Usage: "local host:port to bind a metrics servlet (optional)",
	}
)

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "starts a drand gossip relay process",
	Flags: append(lib.ClientFlags, []cli.Flag{
		idFlag,
		peerWithFlag,
		storeFlag,
		listenFlag,
		metricsFlag,
	}...),
	Action: func(cctx *cli.Context) error {
		if cctx.IsSet(metricsFlag.Name) {
			metricsListener := metrics.Start(cctx.String(metricsFlag.Name), pprof.WithProfile(), nil)
			defer metricsListener.Close()
			if err := metrics.PrivateMetrics.Register(grpc_prometheus.DefaultClientMetrics); err != nil {
				return err
			}
		}

		c, err := lib.Create(cctx, cctx.IsSet(metricsFlag.Name))
		if err != nil {
			return xerrors.Errorf("constructing client: %w", err)
		}

		chainHash := cctx.String(lib.HashFlag.Name)
		if chainHash == "" {
			info, err := c.Info(context.Background())
			if err != nil {
				return xerrors.Errorf("getting chain info: %w", err)
			}
			chainHash = hex.EncodeToString(info.Hash())
		}

		cfg := &lp2p.GossipRelayConfig{
			ChainHash:    chainHash,
			PeerWith:     cctx.StringSlice(peerWithFlag.Name),
			Addr:         cctx.String(listenFlag.Name),
			DataDir:      cctx.String(storeFlag.Name),
			IdentityPath: cctx.String(idFlag.Name),
			Client:       c,
		}
		if _, err := lp2p.NewGossipRelayNode(log.DefaultLogger(), cfg); err != nil {
			return err
		}
		<-chan int(nil)
		return nil
	},
}

var clientCmd = &cli.Command{
	Name:  "client",
	Flags: lib.ClientFlags,
	Action: func(cctx *cli.Context) error {
		c, err := lib.Create(cctx, false)
		if err != nil {
			return xerrors.Errorf("constructing client: %w", err)
		}

		for rand := range c.Watch(context.Background()) {
			log.DefaultLogger().Info("client", "got randomness", "round", rand.Round(), "signature", rand.Signature()[:16])
		}

		return nil
	},
}

var idCmd = &cli.Command{
	Name:  "peerid",
	Usage: "prints the libp2p peer ID or creates one if it does not exist",
	Flags: []cli.Flag{idFlag},
	Action: func(cctx *cli.Context) error {
		priv, err := lp2p.LoadOrCreatePrivKey(cctx.String(idFlag.Name), log.DefaultLogger())
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

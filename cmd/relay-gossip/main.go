package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/urfave/cli/v2"

	"github.com/drand/drand/cmd/client/lib"
	"github.com/drand/drand/common"
	"github.com/drand/drand/log"
	"github.com/drand/drand/lp2p"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/metrics/pprof"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	gitCommit = "none"
	buildDate = "unknown"
)

func main() {
	version := common.GetAppVersion()
	app := &cli.App{
		Name:     "drand-relay-gossip",
		Version:  version.String(),
		Usage:    "pubsub relay for drand randomness beacon",
		Commands: []*cli.Command{runCmd, clientCmd, idCmd},
	}

	// See https://cli.urfave.org/v2/examples/bash-completions/#enabling for how to turn on.
	app.EnableBashCompletion = true

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("drand gossip relay %s (date %v, commit %v)\n", version, buildDate, gitCommit)
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("error: %+v\n", err)
		os.Exit(1)
	}
}

var (
	idFlag = &cli.StringFlag{
		Name:    "identity",
		Usage:   "path to a file containing a libp2p identity (base64 encoded)",
		Value:   "identity.key",
		EnvVars: []string{"DRAND_GOSSIP_IDENTITY"},
	}
	peerWithFlag = &cli.StringSliceFlag{
		Name:    "peer-with",
		Usage:   "peer multiaddr(s) for the relay to direct connect with",
		EnvVars: []string{"DRAND_GOSSIP_PEER_WITH"},
	}
	storeFlag = &cli.StringFlag{
		Name:    "store",
		Usage:   "datastore directory",
		Value:   "./datastore",
		EnvVars: []string{"DRAND_RELAY_STORE"},
	}
	listenFlag = &cli.StringFlag{
		Name:    "listen",
		Usage:   "listening address for libp2p",
		Value:   "/ip4/0.0.0.0/tcp/44544",
		EnvVars: []string{"DRAND_RELAY_LISTEN"},
	}
	metricsFlag = &cli.StringFlag{
		Name:    "metrics",
		Usage:   "local host:port to bind a metrics servlet (optional)",
		EnvVars: []string{"DRAND_RELAY_METRICS"},
	}
)

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "starts a drand gossip relay process",
	Flags: append(lib.ClientFlags, []cli.Flag{
		idFlag,
		peerWithFlag,
		lib.HashListFlag,
		lib.GroupConfListFlag,
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

		if cctx.IsSet(lib.HashFlag.Name) {
			fmt.Printf("--%s is deprecated. Use --%s or --%s instead\n", lib.HashFlag.Name, lib.HashListFlag.Name, lib.GroupConfListFlag.Name)
		}

		switch {
		case cctx.IsSet(lib.GroupConfListFlag.Name) && cctx.IsSet(lib.HashListFlag.Name):
			return fmt.Errorf("only one of --%s and --%s are allowed", lib.GroupConfListFlag.Name, lib.HashListFlag.Name)
		case cctx.IsSet(lib.GroupConfListFlag.Name):
			groupConfs := cctx.StringSlice(lib.GroupConfListFlag.Name)
			for _, groupConf := range groupConfs {
				err := boostrapGossipRelayNode(cctx, groupConf, "")
				if err != nil {
					return err
				}
			}
		case cctx.IsSet(lib.HashListFlag.Name):
			hashes, err := computeHashes(cctx)
			if err != nil {
				return err
			}

			for _, hash := range hashes {
				err := boostrapGossipRelayNode(cctx, "", hash)
				if err != nil {
					return err
				}
			}
		case cctx.IsSet(lib.HashFlag.Name):
			hash := cctx.String(lib.HashFlag.Name)
			if _, err := hex.DecodeString(hash); err != nil {
				return fmt.Errorf("decoding hash %q: %w", hash, err)
			}

			err := boostrapGossipRelayNode(cctx, "", hash)
			if err != nil {
				return err
			}
		default:
			if err := boostrapGossipRelayNode(cctx, "", ""); err != nil {
				return err
			}
		}

		// Wait until we are signaled to shutdown.
		<-cctx.Context.Done()

		return cctx.Context.Err()
	},
}

func boostrapGossipRelayNode(cctx *cli.Context, groupConf, chainHash string) error {
	err := cctx.Set(lib.GroupConfFlag.Name, groupConf)
	if err != nil {
		return err
	}

	err = cctx.Set(lib.HashFlag.Name, chainHash)
	if err != nil {
		return err
	}

	c, err := lib.Create(cctx, cctx.IsSet(metricsFlag.Name))
	if err != nil {
		return fmt.Errorf("constructing client: %w", err)
	}

	chainInfo, err := c.Info(cctx.Context)
	if err != nil {
		return fmt.Errorf("cannot retrieve chain info: %w", err)
	}

	if chainHash == "" {
		chainHash = hex.EncodeToString(chainInfo.Hash())
	}

	// Set the path to be desired 'storage path / beaconID'.
	// This allows running multiple networks via the same beacon.
	dataDir := path.Join(cctx.String(storeFlag.Name), chainInfo.ID)

	cfg := &lp2p.GossipRelayConfig{
		ChainHash:    chainHash,
		PeerWith:     cctx.StringSlice(peerWithFlag.Name),
		Addr:         cctx.String(listenFlag.Name),
		DataDir:      dataDir,
		IdentityPath: cctx.String(idFlag.Name),
		Client:       c,
	}

	l := log.DefaultLogger().With("beaconID", chainInfo.ID)

	_, err = lp2p.NewGossipRelayNode(l, cfg)
	if err != nil {
		err = fmt.Errorf("could not initialize a new gossip relay node %w", err)
	}
	return err
}

func computeHashes(cctx *cli.Context) ([]string, error) {
	hashes := cctx.StringSlice(lib.HashListFlag.Name)
	if len(hashes) == 0 {
		return nil, nil
	}

	for _, hash := range hashes {
		if _, err := hex.DecodeString(hash); err != nil {
			return nil, fmt.Errorf("decoding hash %q: %w", hash, err)
		}
	}

	return hashes, nil
}

var clientCmd = &cli.Command{
	Name:  "client",
	Flags: lib.ClientFlags,
	Action: func(cctx *cli.Context) error {
		c, err := lib.Create(cctx, false)
		if err != nil {
			return fmt.Errorf("constructing client: %w", err)
		}

		for rand := range c.Watch(cctx.Context) {
			log.DefaultLogger().Infow("", "client", "got randomness", "round", rand.Round(), "signature", rand.Signature()[:16])
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
			return fmt.Errorf("loading p2p key: %w", err)
		}
		peerID, err := peer.IDFromPrivateKey(priv)
		if err != nil {
			return fmt.Errorf("computing peerid: %w", err)
		}
		fmt.Printf("%s\n", peerID)
		return nil
	},
}

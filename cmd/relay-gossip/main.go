package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"strings"

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

		var hashMaps []string
		if cctx.IsSet(lib.HashListFlag.Name) {
			hashMaps = cctx.StringSlice(lib.HashListFlag.Name)
		}

		hashFlagSet := cctx.IsSet(lib.HashFlag.Name)
		if hashFlagSet {
			if cctx.IsSet(lib.HashListFlag.Name) {
				return fmt.Errorf("--%s is exclusive with --%s. Use one or the other flag", lib.HashFlag.Name, lib.HashListFlag.Name)
			}

			hashMaps = append(hashMaps, cctx.String(lib.HashFlag.Name))
		}

		groupConfs := computeGroupConfs(cctx)

		hashesMap, err := computeHashesMap(hashMaps, groupConfs)
		if err != nil {
			return err
		}

		if len(groupConfs) >= 1 &&
			len(hashesMap) == 0 {

			for _, groupConf := range groupConfs {
				err = cctx.Set(lib.GroupConfFlag.Name, groupConf)
				if err != nil {
					return fmt.Errorf("setting group-conf %q got: %w", groupConf, err)
				}

				err := boostrapGossipRelayNode(cctx, "")
				if err != nil {
					return err
				}
			}
		}

		for chainHash, groupConf := range hashesMap {
			if err := cctx.Set(lib.HashFlag.Name, chainHash); err != nil {
				return fmt.Errorf("setting client hash: %w", err)
			}

			if groupConf != "" {
				err = cctx.Set(lib.GroupConfFlag.Name, groupConf)
				if err != nil {
					return fmt.Errorf("setting group-conf for hash %q got: %w", chainHash, err)
				}
			}

			if err := boostrapGossipRelayNode(cctx, chainHash); err != nil {
				return err
			}
		}

		// Try and build a default gossip relay using other options that might have been set.
		if len(hashesMap) == 0 &&
			len(groupConfs) == 0 &&
			!hashFlagSet {
			if err := boostrapGossipRelayNode(cctx, ""); err != nil {
				return err
			}
		}

		// Wait indefinitely for our client(s) to run
		<-cctx.Context.Done()

		return cctx.Context.Err()
	},
}

func computeGroupConfs(cctx *cli.Context) []string {
	if !cctx.IsSet(lib.GroupConfFlag.Name) {
		return nil
	}

	groupConfs := cctx.String(lib.GroupConfFlag.Name)
	return strings.Split(groupConfs, ",")
}

func boostrapGossipRelayNode(cctx *cli.Context, chainHash string) error {
	c, err := lib.Create(cctx, cctx.IsSet(metricsFlag.Name))
	if err != nil {
		return fmt.Errorf("constructing client: %w", err)
	}

	chainInfo, err := c.Info(cctx.Context)
	if err != nil {
		return fmt.Errorf("cannot retrieve chain info: %w", err)
	}

	// Set the path to be desired 'storage path / beaconID'.
	// This allows running multiple networks via the same beacon.
	dataDir := path.Join(cctx.String(storeFlag.Name), chainInfo.ID)

	cfg := &lp2p.GossipRelayConfig{
		ChainHash:    chainHash,
		BeaconID:     chainInfo.ID,
		PeerWith:     cctx.StringSlice(peerWithFlag.Name),
		Addr:         cctx.String(listenFlag.Name),
		DataDir:      dataDir,
		IdentityPath: cctx.String(idFlag.Name),
		Client:       c,
	}
	_, err = lp2p.NewGossipRelayNode(log.DefaultLogger(), cfg)
	if err != nil {
		err = fmt.Errorf("could not initialize a new gossip relay node %w", err)
	}
	return err
}

func computeHashesMap(hashMaps, groupConfs []string) (map[string]string, error) {
	hashesMap := make(map[string]string)
	if len(hashMaps) == 0 {
		//nolint:nilnil // We want to return an nil instead of an empty map.
		return nil, nil
	}

	if groupConfs != nil {
		// Here we overload the -group-conf flag and we convert it from String to StringSlice.
		// This is required to provide the correct -group-conf value for the client library.
		if len(groupConfs) > 0 &&
			len(groupConfs) != len(hashMaps) {
			return nil, fmt.Errorf("list of provided hashes is different from list of group-conf files")
		}
	} else {
		for i := 0; i < len(hashMaps); i++ {
			groupConfs = append(groupConfs, "")
		}
	}

	for idx, hash := range hashMaps {
		if _, err := hex.DecodeString(hash); err != nil {
			return nil, fmt.Errorf("decoding hash: %w", err)
		}
		hashesMap[hash] = groupConfs[idx]
	}

	return hashesMap, nil
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

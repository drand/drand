package drand

import (
	"encoding/hex"
	"errors"
	"fmt"
	gonet "net"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/grpc"
	"github.com/drand/drand/core"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
)

func getPublicRandomness(c *cli.Context, lg log.Logger) error {
	if !c.Args().Present() {
		return errors.New("get public command takes a group file as argument")
	}

	certPath := ""
	if c.IsSet(tlsCertFlag.Name) {
		certPath = c.String(tlsCertFlag.Name)
	}

	ids, err := getNodes(c)
	if err != nil {
		return err
	}

	group, err := getGroup(c)
	if err != nil {
		return err
	}
	if group.PublicKey == nil {
		return errors.New("drand: group file must contain the distributed public key")
	}

	info := chain.NewChainInfoWithLogger(lg, group)

	var resp client.Result
	var foundCorrect bool
	for _, id := range ids {
		grpcClient, err := grpc.NewWithLogger(lg, id.Addr, certPath, !id.TLS, info.Hash())
		if err != nil {
			fmt.Fprintf(os.Stderr, "drand: could not connect to %s: %s\n", id.Addr, err)
			break
		}

		resp, err = grpcClient.Get(c.Context, uint64(c.Int(roundFlag.Name)))

		if err == nil {
			foundCorrect = true
			if c.Bool(verboseFlag.Name) {
				fmt.Fprintf(c.App.Writer, "drand: public randomness retrieved from %s\n", id.Addr)
			}
			break
		}
		fmt.Fprintf(os.Stderr, "drand: could not get public randomness from %s: %s\n", id.Addr, err)
	}
	if !foundCorrect {
		return errors.New("drand: could not verify randomness")
	}

	return printJSON(c.App.Writer, resp)
}

func getChainInfo(c *cli.Context, lg log.Logger) error {
	ctx := c.Context
	var err error
	chainHash := make([]byte, 0)
	if c.IsSet(hashInfoNoReq.Name) {
		if chainHash, err = hex.DecodeString(c.String(hashInfoNoReq.Name)); err != nil {
			return fmt.Errorf("invalid chain hash given: %w", err)
		}
	}

	grpcClient := core.NewGrpcClientWithLogger(lg, chainHash)
	if c.IsSet(tlsCertFlag.Name) {
		defaultManager := net.NewCertManagerWithLogger(lg)
		certPath := c.String(tlsCertFlag.Name)
		if err := defaultManager.Add(certPath); err != nil {
			return err
		}
		grpcClient = core.NewGrpcClientFromCertWithLogger(lg, chainHash, defaultManager)
	}
	var ci *chain.Info
	for _, addr := range c.Args().Slice() {
		_, _, err := gonet.SplitHostPort(addr)
		if err != nil {
			return fmt.Errorf("invalid address given: %w", err)
		}
		ci, err = grpcClient.ChainInfo(ctx, net.CreatePeer(addr, !c.Bool("tls-disable")))
		if err == nil {
			break
		}
		fmt.Fprintf(os.Stderr, "drand: error fetching distributed key from %s : %s\n",
			addr, err)
	}
	if ci == nil {
		return errors.New("drand: can't retrieve dist. key from any nodes")
	}
	return printChainInfo(c, ci)
}

func printChainInfo(c *cli.Context, ci *chain.Info) error {
	if c.Bool(hashOnly.Name) {
		fmt.Fprintf(c.App.Writer, "%s\n", hex.EncodeToString(ci.Hash()))
		return nil
	}
	return printJSON(c.App.Writer, ci.ToProto(nil))
}

package drand

import (
	"encoding/hex"
	"errors"
	"fmt"
	gonet "net"
	"os"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/grpc"
	"github.com/drand/drand/core"
	"github.com/drand/drand/net"
	"github.com/urfave/cli/v2"
)

func getPrivateCmd(c *cli.Context) error {
	if !c.Args().Present() {
		return errors.New("get private takes a group file as argument")
	}
	defaultManager := net.NewCertManager()
	if c.IsSet("tls-cert") {
		if err := defaultManager.Add(c.String("tls-cert")); err != nil {
			return err
		}
	}
	ids, err := getNodes(c)
	if err != nil {
		return err
	}
	grpcClient := core.NewGrpcClientFromCert(defaultManager)
	var resp []byte
	for _, public := range ids {
		resp, err = grpcClient.Private(public.Identity)
		if err == nil {
			fmt.Fprintf(output, "drand: successfully retrieved private randomness "+
				"from %s", public.Addr)
			break
		}
		fmt.Fprintf(output, "drand: error contacting node %s: %s", public.Addr, err)
	}
	if resp == nil {
		return errors.New("zero successful contacts with nodes")
	}

	type private struct {
		Randomness []byte
	}

	return printJSON(&private{resp})
}

func getPublicRandomness(c *cli.Context) error {
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

	var resp client.Result
	var foundCorrect bool
	for _, id := range ids {
		grpcClient, err := grpc.New(id.Addr, certPath, !id.TLS)
		if err != nil {
			fmt.Fprintf(os.Stderr, "drand: could not connect to %s: %s", id.Addr, err)
			break
		}

		resp, err = grpcClient.Get(c.Context, uint64(c.Int(roundFlag.Name)))

		if err == nil {
			foundCorrect = true
			if c.Bool(verboseFlag.Name) {
				fmt.Fprintf(output, "drand: public randomness retrieved from %s", id.Addr)
			}
			break
		}
		fmt.Fprintf(os.Stderr, "drand: could not get public randomness from %s: %s", id.Addr, err)
	}
	if !foundCorrect {
		return errors.New("drand: could not verify randomness")
	}

	return printJSON(resp)
}

func getChainInfo(c *cli.Context) error {
	var grpcClient = core.NewGrpcClient()
	if c.IsSet(tlsCertFlag.Name) {
		defaultManager := net.NewCertManager()
		certPath := c.String(tlsCertFlag.Name)
		if err := defaultManager.Add(certPath); err != nil {
			return err
		}
		grpcClient = core.NewGrpcClientFromCert(defaultManager)
	}
	var ci *chain.Info
	for _, addr := range c.Args().Slice() {
		_, _, err := gonet.SplitHostPort(addr)
		if err != nil {
			return fmt.Errorf("invalid address given: %s", err)
		}
		ci, err = grpcClient.ChainInfo(net.CreatePeer(addr, !c.Bool("tls-disable")))
		if err == nil {
			break
		}
		fmt.Fprintf(os.Stderr, "drand: error fetching distributed key from %s : %s",
			addr, err)
	}
	if ci == nil {
		return errors.New("drand: can't retrieve dist. key from all nodes")
	}
	return printChainInfo(c, ci)
}

func printChainInfo(c *cli.Context, ci *chain.Info) error {
	if c.Bool(hashOnly.Name) {
		fmt.Fprintf(output, "%s\n", hex.EncodeToString(ci.Hash()))
		return nil
	}
	return printJSON(ci.ToProto())
}

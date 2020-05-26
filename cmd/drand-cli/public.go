package drand

import (
	"encoding/hex"
	"errors"
	"fmt"
	gonet "net"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/core"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/nikkolasg/slog"
	"github.com/urfave/cli/v2"
)

func getPrivateCmd(c *cli.Context) error {
	if !c.Args().Present() {
		return errors.New("Get private takes a group file as argument.")
	}
	defaultManager := net.NewCertManager()
	if c.IsSet("tls-cert") {
		defaultManager.Add(c.String("tls-cert"))
	}
	ids, err := getNodes(c)
	if err != nil {
		return err
	}
	client := core.NewGrpcClientFromCert(defaultManager)
	var resp []byte
	for _, public := range ids {
		resp, err = client.Private(public.Identity)
		if err == nil {
			slog.Infof("drand: successfully retrieved private randomness "+
				"from %s", public.Addr)
			break
		}
		slog.Infof("drand: error contacting node %s: %s", public.Addr, err)
	}
	if resp == nil {
		return errors.New("zero successful contacts with nodes")
	}

	type private struct {
		Randomness []byte
	}

	printJSON(&private{resp})
	return nil
}

func getPublicRandomness(c *cli.Context) error {
	if !c.Args().Present() {
		return errors.New("Get public command takes a group file as argument.")
	}
	client := core.NewGrpcClient()
	if c.IsSet(tlsCertFlag.Name) {
		defaultManager := net.NewCertManager()
		defaultManager.Add(c.String(tlsCertFlag.Name))
		client = core.NewGrpcClientFromCert(defaultManager)
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
		return errors.New("drand: group file must contain the distributed public key!")
	}

	public := group.PublicKey
	var resp *drand.PublicRandResponse
	var foundCorrect bool
	for _, id := range ids {
		if c.IsSet(roundFlag.Name) {
			resp, err = client.Public(id.Addr, public, id.TLS, c.Int(roundFlag.Name))
		} else {
			resp, err = client.LastPublic(id.Addr, public, id.TLS)
		}
		if err == nil {
			foundCorrect = true
			slog.Infof("drand: public randomness retrieved from %s", id.Addr)
			break
		}
		slog.Printf("drand: could not get public randomness from %s: %s", id.Addr, err)
	}
	if !foundCorrect {
		return errors.New("drand: could not verify randomness")
	}

	printJSON(resp)
	return nil
}

func getChainInfo(c *cli.Context) error {
	var client = core.NewGrpcClient()
	if c.IsSet(tlsCertFlag.Name) {
		defaultManager := net.NewCertManager()
		certPath := c.String(tlsCertFlag.Name)
		defaultManager.Add(certPath)
		client = core.NewGrpcClientFromCert(defaultManager)
	}
	var ci *chain.Info
	for _, addr := range c.Args().Slice() {
		_, _, err := gonet.SplitHostPort(addr)
		if err != nil {
			return fmt.Errorf("invalid address given: %s", err)
		}
		ci, err = client.ChainInfo(net.CreatePeer(addr, !c.Bool("tls-disable")))
		if err == nil {
			break
		}
		slog.Printf("drand: error fetching distributed key from %s : %s",
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

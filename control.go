package main

import (
	"encoding/json"

	"github.com/dedis/drand/core"
	"github.com/dedis/drand/net"
	"github.com/nikkolasg/slog"
	"github.com/urfave/cli"
)

// initDKG indicates to the daemon to start the DKG protocol, as a leader or
// not. The method waits until the DKG protocol finishes or an error occured.
// If the DKG protocol finishes successfully, the beacon randomness loop starts.
func initDKG(c *cli.Context) error {
	groupPath := c.Args().First()
	// still trying to load it ourself now for the moment
	// just to test if it's a valid thing or not
	conf := contextToConfig(c)
	client, err := net.NewControlClient(conf.ControlPort())
	if err != nil {
		slog.Fatalf("drand: error creating control client: %s", err)
	}

	slog.Print("drand: waiting the end of DKG protocol ... " +
		"(you can CTRL-C to not wait)")
	_, err = client.InitDKG(groupPath, c.Bool(leaderFlag.Name))
	if err != nil {
		slog.Fatalf("drand: initdkg %s", err)
	}
	return nil
}

// initReshare indicates to the daemon to start the resharing protocol, as a
// leader or not. The method waits until the resharing protocol finishes or
// an error occured. The "old group" toml is inferred either from the local
// informations that the drand node is keeping (saved in filesystem), and can be
// superseeded by the command line flag "old-group".
// If the DKG protocol finishes successfully, the beacon randomness loop starts.
// NOTE: If the contacted node is not present in the new list of nodes, the
// waiting *can* be infinite in some cases. It's an issue that is low priority
// though.
func initReshare(c *cli.Context) error {
	var isLeader = c.Bool(leaderFlag.Name)
	var oldGroupPath, newGroupPath string

	if c.IsSet(oldGroupFlag.Name) {
		oldGroupPath = c.String(oldGroupFlag.Name)
	} else {
		slog.Print("drand: old group path not specified. Using daemon's own group if possible.")
	}

	if c.NArg() < 1 {
		slog.Fatalf("drand: need new group given as arguments to reshare")
	}
	newGroupPath = c.Args().First()

	conf := contextToConfig(c)
	client, err := net.NewControlClient(conf.ControlPort())
	if err != nil {
		slog.Fatalf("drand: error creating control client: %s", err)
	}

	slog.Print("drand: initiating resharing protocol. Waiting to the end ...")
	_, err = client.InitReshare(oldGroupPath, newGroupPath, isLeader)
	if err != nil {
		slog.Fatalf("drand: error resharing: %s", err)
	}
	return nil
}

func getShare(c *cli.Context) error {
	port := c.String("port")
	if port == "" {
		port = core.DefaultControlPort
	}
	client, err := net.NewControlClient(port)
	if err != nil {
		slog.Fatalf("drand: can't instantiate control client %s", err)
	}
	resp, err := client.GetShare()
	if err != nil {
		slog.Fatalf("drand: could not request the share: %s", err)
	}
	buff, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		slog.Fatal("could not JSON marshal:", err)
	}
	slog.Print(string(buff))
	return nil
}

func pingpong(c *cli.Context) error {
	port := c.String("port")
	if port == "" {
		port = core.DefaultControlPort
	}
	client, err := net.NewControlClient(port)
	if err != nil {
		slog.Fatalf("drand: can't instantiate control client %s", err)
	}
	if err := client.Ping(); err != nil {
		slog.Fatalf("drand: can't ping the daemon ... %s", err)
	}
	slog.Printf("drand daemon is alive on port %s", port)
	return nil
}

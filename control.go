package main

import (
	"encoding/json"
	"io/ioutil"

	"github.com/dedis/drand/core"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/nikkolasg/slog"
	"github.com/urfave/cli"
)

// shareCmd decides whether the command is for a DKG or for a resharing and
// dispatch to the respective sub-commands.
func shareCmd(c *cli.Context) error {
	if !c.Args().Present() {
		slog.Fatal("drand: needs at least one group.toml file argument")
	}

	if c.IsSet(oldGroupFlag.Name) {
		slog.Info("drand: old group file given for resharing protocol")
		return initReshare(c)
	}

	conf := contextToConfig(c)
	fs := key.NewFileStore(conf.ConfigFolder())
	_, errG := fs.LoadGroup()
	_, errS := fs.LoadShare()
	_, errD := fs.LoadDistPublic()
	// XXX place that logic inside core/ directly with only one method
	freshRun := errG != nil || errS != nil || errD != nil
	if freshRun {
		slog.Info("drand: no current distributed key -> running DKG protocol.")
		initDKG(c)
	} else {
		slog.Info("drand: found distributed key -> running resharing protocol.")
		initReshare(c)
	}
	return nil
}

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
		"(you can CTRL-C to not quit waiting)")
	_, err = client.InitDKG(groupPath, c.Bool(leaderFlag.Name))
	if err != nil {
		slog.Fatalf("drand: initdkg %s", err)
	}
	return nil
}

// initReshare indicates to the daemon to start the resharing protocol, as a
// leader or not. The method waits until the resharing protocol finishes or
// an error occured. TInfofhe "old group" toml is inferred either from the local
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
	}
	if oldGroupPath == "" {
		slog.Print("drand: old group path not specified. Using daemon's own group if possible.")
	}

	if c.NArg() < 1 {
		slog.Fatalf("drand: need new group given as arguments to reshare")
	}
	newGroupPath = c.Args().First()

	client := controlClient(c)
	slog.Print("drand: initiating resharing protocol. Waiting to the end ...")
	_, err := client.InitReshare(oldGroupPath, newGroupPath, isLeader)
	if err != nil {
		slog.Fatalf("drand: error resharing: %s", err)
	}
	return nil
}

func getShare(c *cli.Context) error {
	client := controlClient(c)
	resp, err := client.Share()
	if err != nil {
		slog.Fatalf("drand: could not request the share: %s", err)
	}
	printJSON(resp)
	return nil
}

func pingpongCmd(c *cli.Context) error {
	client := controlClient(c)
	if err := client.Ping(); err != nil {
		slog.Fatalf("drand: can't ping the daemon ... %s", err)
	}
	slog.Printf("drand daemon is alive on port %s", controlPort(c))
	return nil
}

func showGroupCmd(c *cli.Context) error {
	client := controlClient(c)
	r, err := client.Group()
	if err != nil {
		slog.Fatalf("drand: error asking for group file")
	}
	if c.IsSet(outFlag.Name) {
		filePath := c.String(outFlag.Name)
		ioutil.WriteFile(filePath, []byte(r.Group), 0777)
		slog.Printf("group file written to %s", filePath)
	} else {
		slog.Printf("\n\n%s", r.Group)
	}
	return nil
}

func showCokeyCmd(c *cli.Context) error {
	client := controlClient(c)
	resp, err := client.CollectiveKey()
	if err != nil {
		slog.Fatalf("drand: could not request drand.cokey: %s", err)
	}
	printJSON(resp)
	return nil
}

func showPrivateCmd(c *cli.Context) error {
	client := controlClient(c)
	resp, err := client.PrivateKey()
	if err != nil {
		slog.Fatalf("drand: could not request drand.private: %s", err)
	}
	printJSON(resp)
	return nil
}

func showPublicCmd(c *cli.Context) error {
	client := controlClient(c)
	resp, err := client.PublicKey()
	if err != nil {
		slog.Fatalf("drand: could not request drand.public: %s", err)
	}
	printJSON(resp)
	return nil
}

func showShareCmd(c *cli.Context) error {
	client := controlClient(c)
	resp, err := client.Share()
	if err != nil {
		slog.Fatalf("drand: could not request drand.share: %s", err)
	}
	printJSON(resp)
	return nil
}

func controlPort(c *cli.Context) string {
	port := c.String("control")
	if port == "" {
		port = core.DefaultControlPort
	}
	return port
}

func controlClient(c *cli.Context) *net.ControlClient {
	port := controlPort(c)
	client, err := net.NewControlClient(port)
	if err != nil {
		slog.Fatalf("drand: can't instantiate control client: %s", err)
	}
	return client
}

func printJSON(j interface{}) {
	buff, err := json.MarshalIndent(j, "", "    ")
	if err != nil {
		slog.Fatalf("drand: could not JSON marshal: %s", err)
	}
	slog.Print(string(buff))
}

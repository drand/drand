package main

import (
	"fmt"
	"os"
	"time"

	"github.com/drand/drand/core"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	control "github.com/drand/drand/protobuf/drand"

	json "github.com/nikkolasg/hexjson"
	"github.com/urfave/cli/v2"
)

func shareCmd(c *cli.Context) error {
	isResharing := c.IsSet(transitionFlag.Name) || c.IsSet(oldGroupFlag.Name)
	isLeader := c.Bool(leaderFlag.Name)

	var connectPeer net.Peer
	if !isLeader {
		if !c.IsSet(connectFlag.Name) {
			fatal("need to the address of the coordinator to create the group file")
		}
		coordAddress := c.String(connectFlag.Name)
		isTls := !c.IsSet(insecureFlag.Name)
		connectPeer = net.CreatePeer(coordAddress, isTls)
	}

	nodes := c.Int(shareNodeFlag.Name)
	thr := c.Int(thresholdFlag.Name)
	secret := c.String(secretFlag.Name)
	var timeout = core.DefaultDKGTimeout
	if c.IsSet(timeoutFlag.Name) {
		var err error
		str := c.String(timeoutFlag.Name)
		timeout, err = time.ParseDuration(str)
		if err != nil {
			fatal("dkg timeout duration incorrect:", err)
		}
	}

	conf := contextToConfig(c)
	client, err := net.NewControlClient(conf.ControlPort())
	if err != nil {
		fatal("could not create client: %v", err)
	}

	var groupP *control.GroupPacket
	var shareErr error
	if !isResharing {
		if c.IsSet(userEntropyOnlyFlag.Name) && !c.IsSet(sourceFlag.Name) {
			fmt.Print("drand: userEntropyOnly needs to be used with the source flag, which is not specified here. userEntropyOnly flag is ignored.")
		}
		entropyInfo := entropyInfoFromReader(c)
		if isLeader {
			if !c.IsSet(periodFlag.Name) {
				fatal("leader flag indicated requires the beacon period flag as well")
			}
			periodStr := c.String(periodFlag.Name)
			period, err := time.ParseDuration(periodStr)
			if err != nil {
				fatal("period given is invalid: %v", err)
			}

			offset := int(core.DefaultGenesisOffset.Seconds())
			if c.IsSet(beaconOffset.Name) {
				offset = c.Int(beaconOffset.Name)
			}
			fmt.Println("Initiating the DKG as a leader")
			fmt.Println("You can stop the command at any point. If so, the group " +
				"file will not be written out to the specified output. To get the" +
				"group file once the setup phase is done, you can run the `drand show" +
				"group` command")
			groupP, shareErr = client.InitDKGLeader(nodes, thr, period, timeout, entropyInfo, secret, offset)
			fmt.Println(" --- got err", shareErr, "group", groupP)
		} else {
			fmt.Println("Participating to the setup of the DKG")
			groupP, shareErr = client.InitDKG(connectPeer, nodes, thr, timeout, entropyInfo, secret)
			fmt.Println(" --- got err", shareErr, "group", groupP)
		}
	} else {
		// resharing case needs the previous group
		var oldPath string
		if c.IsSet(transitionFlag.Name) {
			// daemon will try to the load the one stored
			oldPath = ""
		} else if c.IsSet(oldGroupFlag.Name) {
			var oldGroup = new(key.Group)
			if err := key.Load(c.String(oldGroupFlag.Name), oldGroup); err != nil {
				fatal("could not load drand from path", err)
			}
			oldPath = c.String(oldGroupFlag.Name)
		}

		if isLeader {
			offset := int(core.DefaultResharingOffset.Seconds())
			if c.IsSet(beaconOffset.Name) {
				offset = c.Int(beaconOffset.Name)
			}
			fmt.Println("Initiating the resharing as a leader")
			groupP, shareErr = client.InitReshareLeader(nodes, thr, timeout, secret, oldPath, offset)
		} else {
			fmt.Println("Participating to the resharing")
			groupP, shareErr = client.InitReshare(connectPeer, nodes, thr, timeout, secret, oldPath)
		}
	}
	if shareErr != nil {
		fatal("error setting up the network: %v", err)
	}
	group, err := key.GroupFromProto(groupP)
	if err != nil {
		fatal("error interpreting the group from protobuf: %v", err)
	}
	groupOut(c, group)
	return nil
}

func getShare(c *cli.Context) error {
	client := controlClient(c)
	resp, err := client.Share()
	if err != nil {
		fatal("drand: could not request the share: %s", err)
	}
	printJSON(resp)
	return nil
}

func pingpongCmd(c *cli.Context) error {
	client := controlClient(c)
	if err := client.Ping(); err != nil {
		fatal("drand: can't ping the daemon ... %s", err)
	}
	fmt.Printf("drand daemon is alive on port %s", controlPort(c))
	return nil
}

func showGroupCmd(c *cli.Context) error {
	client := controlClient(c)
	r, err := client.GroupFile()
	if err != nil {
		fatal("drand: fetching group file error: %s", err)
	}
	group, err := key.GroupFromProto(r)
	if err != nil {
		return err
	}
	groupOut(c, group)
	return nil
}

func showCokeyCmd(c *cli.Context) error {
	client := controlClient(c)
	resp, err := client.CollectiveKey()
	if err != nil {
		fatal("drand: could not request drand.cokey: %s", err)
	}
	printJSON(resp)
	return nil
}

func showPrivateCmd(c *cli.Context) error {
	client := controlClient(c)
	resp, err := client.PrivateKey()
	if err != nil {
		fatal("drand: could not request drand.private: %s", err)
	}
	printJSON(resp)
	return nil
}

func showPublicCmd(c *cli.Context) error {
	client := controlClient(c)
	resp, err := client.PublicKey()
	if err != nil {
		fatal("drand: could not request drand.public: %s", err)
	}

	printJSON(resp)
	return nil
}

func showShareCmd(c *cli.Context) error {
	client := controlClient(c)
	resp, err := client.Share()
	if err != nil {
		fatal("drand: could not request drand.share: %s", err)
	}

	printJSON(resp)
	return nil
}

func controlPort(c *cli.Context) string {
	port := c.String(controlFlag.Name)
	if port == "" {
		port = core.DefaultControlPort
	}
	return port
}

func controlClient(c *cli.Context) *net.ControlClient {
	port := controlPort(c)
	client, err := net.NewControlClient(port)
	if err != nil {
		fatal("drand: can't instantiate control client: %s", err)
	}
	return client
}

func printJSON(j interface{}) {
	buff, err := json.MarshalIndent(j, "", "    ")
	if err != nil {
		fatal("drand: could not JSON marshal: %s", err)
	}
	fmt.Println(string(buff))
}

func entropyInfoFromReader(c *cli.Context) *control.EntropyInfo {
	if c.IsSet(sourceFlag.Name) {
		_, err := os.Lstat(c.String(sourceFlag.Name))
		if err != nil {
			fatal("drand: cannot use given entropy source: %s", err)
		}
		source := c.String(sourceFlag.Name)
		ei := &control.EntropyInfo{
			Script:   source,
			UserOnly: c.Bool(userEntropyOnlyFlag.Name),
		}
		return ei
	}
	return nil
}

package drand

import (
	"fmt"
	"os"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/core"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	control "github.com/drand/drand/protobuf/drand"

	json "github.com/nikkolasg/hexjson"
	"github.com/urfave/cli/v2"
)

const minimumShareSecretLength = 32

func shareCmd(c *cli.Context) error {
	if c.IsSet(transitionFlag.Name) || c.IsSet(oldGroupFlag.Name) {
		return reshareCmd(c)
	}
	isLeader := c.Bool(leaderFlag.Name)

	var connectPeer net.Peer
	if !isLeader {
		if !c.IsSet(connectFlag.Name) {
			return fmt.Errorf("need to the address of the coordinator to create the group file")
		}
		coordAddress := c.String(connectFlag.Name)
		isTLS := !c.IsSet(insecureFlag.Name)
		connectPeer = net.CreatePeer(coordAddress, isTLS)
	}
	if isLeader && !(c.IsSet(thresholdFlag.Name) && c.IsSet(shareNodeFlag.Name)) {
		return fmt.Errorf("leader needs to specify --nodes and --threshold for sharing")
	}

	nodes := c.Int(shareNodeFlag.Name)
	thr, err := getThreshold(c)
	if err != nil {
		return err
	}
	secret := c.String(secretFlag.Name)
	if len(secret) < minimumShareSecretLength {
		return fmt.Errorf("secret is insecure. Should be at least %d characters", minimumShareSecretLength)
	}
	timeout, err := getTimeout(c)
	if err != nil {
		return err
	}

	conf := contextToConfig(c)
	ctrlClient, err := net.NewControlClient(conf.ControlPort())
	if err != nil {
		return fmt.Errorf("could not create client: %v", err)
	}

	var groupP *control.GroupPacket
	var shareErr error
	if c.IsSet(userEntropyOnlyFlag.Name) && !c.IsSet(sourceFlag.Name) {
		fmt.Print("drand: userEntropyOnly needs to be used with the source flag, which is not specified here. userEntropyOnly flag is ignored.")
	}
	entropyInfo, err := entropyInfoFromReader(c)
	if err != nil {
		return fmt.Errorf("error getting entropy source: %s", err)
	}
	if isLeader {
		if !c.IsSet(periodFlag.Name) {
			return fmt.Errorf("leader flag indicated requires the beacon period flag as well")
		}
		periodStr := c.String(periodFlag.Name)
		period, err := time.ParseDuration(periodStr)
		if err != nil {
			return fmt.Errorf("period given is invalid: %v", err)
		}

		offset := int(core.DefaultGenesisOffset.Seconds())
		if c.IsSet(beaconOffset.Name) {
			offset = c.Int(beaconOffset.Name)
		}
		fmt.Fprintln(output, "Initiating the DKG as a leader")
		fmt.Fprintln(output, "You can stop the command at any point. If so, the group "+
			"file will not be written out to the specified output. To get the"+
			"group file once the setup phase is done, you can run the `drand show"+
			"group` command")
		groupP, shareErr = ctrlClient.InitDKGLeader(nodes, thr, period, timeout, entropyInfo, secret, offset)
		fmt.Fprintln(output, " --- got err", shareErr, "group", groupP)
	} else {
		fmt.Fprintln(output, "Participating to the setup of the DKG")
		groupP, shareErr = ctrlClient.InitDKG(connectPeer, entropyInfo, secret)
		fmt.Fprintln(output, " --- got err", shareErr, "group", groupP)
	}

	if shareErr != nil {
		return fmt.Errorf("error setting up the network: %v", err)
	}
	group, err := key.GroupFromProto(groupP)
	if err != nil {
		return fmt.Errorf("error interpreting the group from protobuf: %v", err)
	}
	return groupOut(c, group)
}

func getTimeout(c *cli.Context) (timeout time.Duration, err error) {
	if c.IsSet(timeoutFlag.Name) {
		str := c.String(timeoutFlag.Name)
		return time.ParseDuration(str)
	}
	return core.DefaultDKGTimeout, nil
}

func reshareCmd(c *cli.Context) error {
	isLeader := c.Bool(leaderFlag.Name)

	var connectPeer net.Peer
	if !isLeader {
		if !c.IsSet(connectFlag.Name) {
			return fmt.Errorf("need to the address of the coordinator to create the group file")
		}
		coordAddress := c.String(connectFlag.Name)
		isTLS := !c.IsSet(insecureFlag.Name)
		connectPeer = net.CreatePeer(coordAddress, isTLS)
	}
	if isLeader && !(c.IsSet(thresholdFlag.Name) && c.IsSet(shareNodeFlag.Name)) {
		return fmt.Errorf("leader needs to specify --nodes and --threshold for sharing")
	}

	nodes := c.Int(shareNodeFlag.Name)
	thr, err := getThreshold(c)
	if err != nil {
		return err
	}
	secret := c.String(secretFlag.Name)
	if len(secret) < minimumShareSecretLength {
		return fmt.Errorf("secret is insecure. Should be at least %d characters", minimumShareSecretLength)
	}
	timeout, err := getTimeout(c)
	if err != nil {
		return err
	}

	conf := contextToConfig(c)
	ctrlClient, err := net.NewControlClient(conf.ControlPort())
	if err != nil {
		return fmt.Errorf("could not create client: %v", err)
	}

	var groupP *control.GroupPacket
	var shareErr error

	// resharing case needs the previous group
	var oldPath string
	if c.IsSet(transitionFlag.Name) {
		// daemon will try to the load the one stored
		oldPath = ""
	} else if c.IsSet(oldGroupFlag.Name) {
		var oldGroup = new(key.Group)
		if err := key.Load(c.String(oldGroupFlag.Name), oldGroup); err != nil {
			return fmt.Errorf("could not load drand from path: %s", err)
		}
		oldPath = c.String(oldGroupFlag.Name)
	}

	if isLeader {
		offset := int(core.DefaultResharingOffset.Seconds())
		if c.IsSet(beaconOffset.Name) {
			offset = c.Int(beaconOffset.Name)
		}
		fmt.Fprintln(output, "Initiating the resharing as a leader")
		groupP, shareErr = ctrlClient.InitReshareLeader(nodes, thr, timeout, secret, oldPath, offset)
	} else {
		fmt.Fprintln(output, "Participating to the resharing")
		groupP, shareErr = ctrlClient.InitReshare(connectPeer, secret, oldPath)
	}
	if shareErr != nil {
		return fmt.Errorf("error setting up the network: %v", err)
	}
	group, err := key.GroupFromProto(groupP)
	if err != nil {
		return fmt.Errorf("error interpreting the group from protobuf: %v", err)
	}
	return groupOut(c, group)
}

func pingpongCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}
	if err := client.Ping(); err != nil {
		return fmt.Errorf("drand: can't ping the daemon ... %s", err)
	}
	fmt.Fprintf(output, "drand daemon is alive on port %s", controlPort(c))
	return nil
}

func showGroupCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}
	r, err := client.GroupFile()
	if err != nil {
		return fmt.Errorf("fetching group file error: %s", err)
	}
	group, err := key.GroupFromProto(r)
	if err != nil {
		return err
	}
	return groupOut(c, group)
}

func showChainInfo(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}
	resp, err := client.ChainInfo()
	if err != nil {
		return fmt.Errorf("could not request chain info: %s", err)
	}
	ci, err := chain.InfoFromProto(resp)
	if err != nil {
		return fmt.Errorf("could not get correct chain info: %s", err)
	}
	return printChainInfo(c, ci)
}

func showPrivateCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}
	resp, err := client.PrivateKey()
	if err != nil {
		return fmt.Errorf("could not request drand.private: %s", err)
	}
	return printJSON(resp)
}

func showPublicCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}
	resp, err := client.PublicKey()
	if err != nil {
		return fmt.Errorf("drand: could not request drand.public: %s", err)
	}
	return printJSON(resp)
}

func showShareCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}
	resp, err := client.Share()
	if err != nil {
		return fmt.Errorf("could not request drand.share: %s", err)
	}
	return printJSON(resp)
}

func controlPort(c *cli.Context) string {
	port := c.String(controlFlag.Name)
	if port == "" {
		port = core.DefaultControlPort
	}
	return port
}

func controlClient(c *cli.Context) (*net.ControlClient, error) {
	port := controlPort(c)
	client, err := net.NewControlClient(port)
	if err != nil {
		return nil, fmt.Errorf("can't instantiate control client: %s", err)
	}
	return client, nil
}

func printJSON(j interface{}) error {
	buff, err := json.MarshalIndent(j, "", "    ")
	if err != nil {
		return fmt.Errorf("could not JSON marshal: %s", err)
	}
	fmt.Fprintln(output, string(buff))
	return nil
}

func entropyInfoFromReader(c *cli.Context) (*control.EntropyInfo, error) {
	if c.IsSet(sourceFlag.Name) {
		_, err := os.Lstat(c.String(sourceFlag.Name))
		if err != nil {
			return nil, fmt.Errorf("cannot use given entropy source: %s", err)
		}
		source := c.String(sourceFlag.Name)
		ei := &control.EntropyInfo{
			Script:   source,
			UserOnly: c.Bool(userEntropyOnlyFlag.Name),
		}
		return ei, nil
	}
	return nil, nil
}
func selfSign(c *cli.Context) error {
	conf := contextToConfig(c)
	fs := key.NewFileStore(conf.ConfigFolder())
	pair, err := fs.LoadKeyPair()
	if err != nil {
		return fmt.Errorf("loading private/public: %s", err)
	}
	if pair.Public.ValidSignature() == nil {
		fmt.Fprintln(output, "Public identity already self signed.")
		return nil
	}
	pair.SelfSign()
	if err := fs.SaveKeyPair(pair); err != nil {
		return fmt.Errorf("saving identity: %s", err)
	}
	fmt.Fprintln(output, "Public identity self signed")
	fmt.Fprintln(output, printJSON(pair.Public.TOML()))
	return nil
}

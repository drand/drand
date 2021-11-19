package drand

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/drand/drand/core/migration"

	"github.com/drand/drand/common/scheme"

	"github.com/briandowns/spinner"
	"github.com/drand/drand/chain"
	"github.com/drand/drand/core"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand" //nolint:stylecheck

	control "github.com/drand/drand/protobuf/drand" //nolint:stylecheck

	json "github.com/nikkolasg/hexjson"
	"github.com/urfave/cli/v2"
)

const minimumShareSecretLength = 32

type shareArgs struct {
	secret    string
	isTLS     bool
	timeout   time.Duration
	threshold int
	entropy   *control.EntropyInfo
	force     bool
	conf      *core.Config
}

func (s *shareArgs) loadSecret(c *cli.Context) error {
	secret := os.Getenv("DRAND_SHARE_SECRET")
	if c.IsSet(secretFlag.Name) {
		bytes, err := os.ReadFile(c.String(secretFlag.Name))
		if err != nil {
			return err
		}
		secret = string(bytes)
	}
	if secret == "" {
		return fmt.Errorf("no secret specified for share")
	}
	if len(secret) < minimumShareSecretLength {
		return fmt.Errorf("secret is insecure. Should be at least %d characters", minimumShareSecretLength)
	}
	s.secret = secret
	return nil
}

func getShareArgs(c *cli.Context) (*shareArgs, error) {
	var err error
	args := new(shareArgs)
	if err := args.loadSecret(c); err != nil {
		return nil, err
	}

	args.isTLS = !c.IsSet(insecureFlag.Name)

	args.timeout, err = getTimeout(c)
	if err != nil {
		return nil, err
	}

	args.threshold, err = getThreshold(c)
	if err != nil {
		return nil, err
	}

	args.force = c.Bool(forceFlag.Name)

	if c.IsSet(userEntropyOnlyFlag.Name) && !c.IsSet(sourceFlag.Name) {
		fmt.Print("drand: userEntropyOnly needs to be used with the source flag, which is not specified here. userEntropyOnly flag is ignored.")
	}
	args.entropy, err = entropyInfoFromReader(c)
	if err != nil {
		return nil, fmt.Errorf("error getting entropy source: %w", err)
	}

	args.conf = contextToConfig(c)

	return args, nil
}

func shareCmd(c *cli.Context) error {
	if c.IsSet(transitionFlag.Name) || c.IsSet(oldGroupFlag.Name) {
		return reshareCmd(c)
	}

	if c.Bool(leaderFlag.Name) {
		return leadShareCmd(c)
	}

	args, err := getShareArgs(c)
	if err != nil {
		return err
	}
	if !c.IsSet(connectFlag.Name) {
		return fmt.Errorf("need to the address of the coordinator to create the group file")
	}
	coordAddress := c.String(connectFlag.Name)
	connectPeer := net.CreatePeer(coordAddress, args.isTLS)

	ctrlClient, err := net.NewControlClient(args.conf.ControlPort())
	if err != nil {
		return fmt.Errorf("could not create client: %v", err)
	}

	beaconID := c.String(beaconIDFlag.Name)

	fmt.Fprintf(output, "Participating to the setup of the DKG. Beacon ID: [%s] \n", beaconID)
	groupP, shareErr := ctrlClient.InitDKG(connectPeer, args.entropy, args.secret, beaconID)

	if shareErr != nil {
		return fmt.Errorf("error setting up the network: %v", shareErr)
	}
	group, err := key.GroupFromProto(groupP)
	if err != nil {
		return fmt.Errorf("error interpreting the group from protobuf: %v", err)
	}
	return groupOut(c, group)
}

func leadShareCmd(c *cli.Context) error {
	if !c.IsSet(thresholdFlag.Name) || !c.IsSet(shareNodeFlag.Name) {
		return fmt.Errorf("leader needs to specify --nodes and --threshold for sharing")
	}

	args, err := getShareArgs(c)
	if err != nil {
		return err
	}

	nodes := c.Int(shareNodeFlag.Name)
	if nodes <= 1 {
		fmt.Fprintln(output, "Warning: less than 2 nodes is an unsupported, degenerate mode.")
	}

	ctrlClient, err := net.NewControlClient(args.conf.ControlPort())
	if err != nil {
		return fmt.Errorf("could not create client: %v", err)
	}

	if !c.IsSet(periodFlag.Name) {
		return fmt.Errorf("leader flag indicated requires the beacon period flag as well")
	}
	periodStr := c.String(periodFlag.Name)

	period, err := time.ParseDuration(periodStr)
	if err != nil {
		return fmt.Errorf("period given is invalid: %v", err)
	}

	catchupPeriod := time.Duration(0)
	catchupPeriodStr := c.String(catchupPeriodFlag.Name)
	if catchupPeriod, err = time.ParseDuration(catchupPeriodStr); err != nil {
		return fmt.Errorf("catchup period given is invalid: %v", err)
	}

	var sch scheme.Scheme
	if sch, err = scheme.GetSchemeByIDWithDefault(c.String(schemeFlag.Name)); err != nil {
		return fmt.Errorf("scheme given is invalid: %v", err)
	}

	offset := int(core.DefaultGenesisOffset.Seconds())
	if c.IsSet(beaconOffset.Name) {
		offset = c.Int(beaconOffset.Name)
	}

	beaconID := c.String(beaconIDFlag.Name)

	str1 := fmt.Sprintf("Initiating the DKG as a leader. Beacon ID: [%s]", beaconID)

	fmt.Fprintln(output, str1)
	fmt.Fprintln(output, "You can stop the command at any point. If so, the group "+
		"file will not be written out to the specified output. To get the "+
		"group file once the setup phase is done, you can run the `drand show "+
		"group` command\n")
	groupP, shareErr := ctrlClient.InitDKGLeader(nodes, args.threshold, period,
		catchupPeriod, args.timeout, args.entropy, args.secret, offset, sch.ID, beaconID)

	if shareErr != nil {
		return fmt.Errorf("error setting up the network: %v", shareErr)
	}
	group, err := key.GroupFromProto(groupP)
	if err != nil {
		return fmt.Errorf("error interpreting the group from protobuf: %v", err)
	}
	return groupOut(c, group)
}

func reshareCmd(c *cli.Context) error {
	if c.Bool(leaderFlag.Name) {
		return leadReshareCmd(c)
	}

	args, err := getShareArgs(c)
	if err != nil {
		return err
	}

	if !c.IsSet(connectFlag.Name) {
		return fmt.Errorf("need to the address of the coordinator to create the group file")
	}
	coordAddress := c.String(connectFlag.Name)
	connectPeer := net.CreatePeer(coordAddress, args.isTLS)

	ctrlClient, err := net.NewControlClient(args.conf.ControlPort())
	if err != nil {
		return fmt.Errorf("could not create client: %v", err)
	}

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

	beaconID := c.String(beaconIDFlag.Name)

	fmt.Fprintf(output, "Participating to the resharing. Beacon ID: [%s] \n", beaconID)

	groupP, shareErr := ctrlClient.InitReshare(connectPeer, args.secret, oldPath, args.force, beaconID)
	if shareErr != nil {
		return fmt.Errorf("error setting up the network: %v", shareErr)
	}
	group, err := key.GroupFromProto(groupP)
	if err != nil {
		return fmt.Errorf("error interpreting the group from protobuf: %v", err)
	}
	return groupOut(c, group)
}

func leadReshareCmd(c *cli.Context) error {
	args, err := getShareArgs(c)
	if err != nil {
		return err
	}

	if !c.IsSet(thresholdFlag.Name) || !c.IsSet(shareNodeFlag.Name) {
		return fmt.Errorf("leader needs to specify --nodes and --threshold for sharing")
	}

	nodes := c.Int(shareNodeFlag.Name)

	ctrlClient, err := net.NewControlClient(args.conf.ControlPort())
	if err != nil {
		return fmt.Errorf("could not create client: %v", err)
	}

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

	offset := int(core.DefaultResharingOffset.Seconds())
	if c.IsSet(beaconOffset.Name) {
		offset = c.Int(beaconOffset.Name)
	}
	catchupPeriod := time.Duration(-1)
	if c.IsSet(catchupPeriodFlag.Name) {
		catchupPeriodStr := c.String(catchupPeriodFlag.Name)
		if catchupPeriod, err = time.ParseDuration(catchupPeriodStr); err != nil {
			return fmt.Errorf("catchup period given is invalid: %v", err)
		}
	}

	beaconID := c.String(beaconIDFlag.Name)

	fmt.Fprintf(output, "Initiating the resharing as a leader. Beacon ID: [%s] \n", beaconID)
	groupP, shareErr := ctrlClient.InitReshareLeader(nodes, args.threshold, args.timeout,
		catchupPeriod, args.secret, oldPath, offset, beaconID)

	if shareErr != nil {
		return fmt.Errorf("error setting up the network: %v", shareErr)
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

func remoteStatusCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}
	ips := c.Args().Slice()
	isTLS := !c.IsSet(insecureFlag.Name)
	addresses := make([]*drand.Address, len(ips))
	for i := 0; i < len(ips); i++ {
		addresses[i] = &drand.Address{
			Address: ips[i],
			Tls:     isTLS,
		}
	}
	resp, err := client.RemoteStatus(c.Context, addresses)
	if err != nil {
		return err
	}
	// set default value for all keys so json output outputs something for all
	// keys
	defaultMap := make(map[string]*control.StatusResponse)
	for _, addr := range addresses {
		if resp, ok := resp[addr.GetAddress()]; !ok {
			defaultMap[addr.GetAddress()] = nil
		} else {
			defaultMap[addr.GetAddress()] = resp
		}
	}

	if c.IsSet(jsonFlag.Name) {
		str, err := json.Marshal(defaultMap)
		if err != nil {
			return fmt.Errorf("cannot marshal the response ... %s", err)
		}
		fmt.Fprintf(output, "%s \n", string(str))
	} else {
		for addr, resp := range defaultMap {
			fmt.Fprintf(output, "Status of node %s\n", addr)
			if resp == nil {
				fmt.Fprintf(output, "\t- NO STATUS; can't connect\n")
			} else {
				fmt.Fprintf(output, "%s\n", core.StatusResponseToString(resp))
			}
		}
	}
	return nil
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

func statusCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}
	resp, err := client.Status()
	if err != nil {
		return fmt.Errorf("drand: can't get the status of the daemon ... %s", err)
	}

	if c.IsSet(jsonFlag.Name) {
		str, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("cannot marshal the response ... %s", err)
		}
		fmt.Fprintf(output, "%s \n", string(str))
	} else {
		fmt.Fprintf(output, "drand daemon is alive on port %s and its status is: \n", controlPort(c))
		fmt.Fprintf(output, "%s \n", core.StatusResponseToString(resp))
	}

	return nil
}

func migrateCmd(c *cli.Context) error {
	conf := contextToConfig(c)
	if err := migration.MigrateSBFolderStructure(conf.ConfigFolder()); err != nil {
		return fmt.Errorf("cannot migrate folder structure, please try again. err: %s", err)
	}

	fmt.Fprintf(output, "folder structure is now ready to support multi-beacon drand\n")
	return nil
}

func schemesCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}
	resp, err := client.ListSchemes()
	if err != nil {
		return fmt.Errorf("drand: can't get the list of scheme ids availables ... %s", err)
	}

	fmt.Fprintf(output, "Drand supports the following list of schemes: \n")

	for i, id := range resp.Ids {
		fmt.Fprintf(output, "%d) %s \n", i, id)
	}

	fmt.Fprintf(output, "\nChoose one of them and set it on --scheme flag \n")
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

func backupDBCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}
	err = client.BackupDB(c.String(outFlag.Name))
	if err != nil {
		return fmt.Errorf("could not back up: %s", err)
	}
	return nil
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
	beaconID := getBeaconID(c)

	fs := key.NewFileStore(conf.ConfigFolderMB(), beaconID)
	pair, err := fs.LoadKeyPair()

	if err != nil {
		return fmt.Errorf("beacon id [%s] - loading private/public: %s", beaconID, err)
	}
	if pair.Public.ValidSignature() == nil {
		fmt.Fprintf(output, "beacon id [%s] - public identity already self signed.\n", beaconID)
		return nil
	}

	pair.SelfSign()
	if err := fs.SaveKeyPair(pair); err != nil {
		return fmt.Errorf("beacon id [%s] - saving identity: %s", beaconID, err)
	}

	fmt.Fprintf(output, "beacon id [%s] - Public identity self signed", beaconID)
	fmt.Fprintln(output, printJSON(pair.Public.TOML()))
	return nil
}

const refreshRate = 1000 * time.Millisecond

func followCmd(c *cli.Context) error {
	ctrlClient, err := controlClient(c)
	if err != nil {
		return fmt.Errorf("unable to create control client: %s", err)
	}

	addrs := strings.Split(c.String(syncNodeFlag.Name), ",")
	channel, errCh, err := ctrlClient.StartFollowChain(
		c.Context,
		c.String(hashInfoFlag.Name),
		addrs,
		!c.Bool(insecureFlag.Name),
		uint64(c.Int(upToFlag.Name)),
		c.String(beaconIDFlag.Name))

	if err != nil {
		return fmt.Errorf("error asking to follow chain: %s", err)
	}

	var current uint64
	var target uint64
	s := spinner.New(spinner.CharSets[9], refreshRate)
	s.PreUpdate = func(spin *spinner.Spinner) {
		curr := atomic.LoadUint64(&current)
		tar := atomic.LoadUint64(&target)
		spin.Suffix = fmt.Sprintf("  synced round up to %d "+
			"- current target %d"+
			"\t--> %.3f %% - "+
			"Waiting on new rounds...", curr, tar, 100*float64(curr)/float64(tar))
	}
	s.FinalMSG = "Follow stopped"
	s.Start()
	defer s.Stop()
	for {
		select {
		case progress := <-channel:
			atomic.StoreUint64(&current, progress.Current)
			atomic.StoreUint64(&target, progress.Target)
		case err := <-errCh:
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("errror on following the chain: %s", err)
		}
	}
}

package drand

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/drand/drand/common"

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
	force     bool
	isTLS     bool
	threshold int
	timeout   time.Duration
	secret    string
	entropy   *control.EntropyInfo
	conf      *core.Config
}

type beaconIDsStatuses struct {
	Beacons map[string]*control.StatusResponse `json:"beacons"`
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

	if err := checkArgs(c); err != nil {
		return nil, err
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
		return fmt.Errorf("could not create client: %w", err)
	}

	beaconID := getBeaconID(c)

	fmt.Fprintf(output, "Participating to the setup of the DKG. Beacon ID: [%s] \n", beaconID)
	groupP, shareErr := ctrlClient.InitDKG(connectPeer, args.entropy, args.secret, beaconID)

	if shareErr != nil {
		return fmt.Errorf("error setting up the network: %w", shareErr)
	}
	group, err := key.GroupFromProto(groupP)
	if err != nil {
		return fmt.Errorf("error interpreting the group from protobuf: %w", err)
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
		return fmt.Errorf("could not create client: %w", err)
	}

	if !c.IsSet(periodFlag.Name) {
		return fmt.Errorf("leader flag indicated requires the beacon period flag as well")
	}
	periodStr := c.String(periodFlag.Name)

	period, err := time.ParseDuration(periodStr)
	if err != nil {
		return fmt.Errorf("period given is invalid: %w", err)
	}

	var catchupPeriod time.Duration
	catchupPeriodStr := c.String(catchupPeriodFlag.Name)
	if catchupPeriod, err = time.ParseDuration(catchupPeriodStr); err != nil {
		return fmt.Errorf("catchup period given is invalid: %w", err)
	}

	var sch scheme.Scheme
	if sch, err = scheme.GetSchemeByIDWithDefault(c.String(schemeFlag.Name)); err != nil {
		return fmt.Errorf("scheme given is invalid: %w", err)
	}

	offset := int(core.DefaultGenesisOffset.Seconds())
	if c.IsSet(beaconOffset.Name) {
		offset = c.Int(beaconOffset.Name)
	}

	beaconID := getBeaconID(c)

	str1 := fmt.Sprintf("Initiating the DKG as a leader. Beacon ID: [%s]", beaconID)

	fmt.Fprintln(output, str1)
	fmt.Fprintln(output, "You can stop the command at any point. If so, the group "+
		"file will not be written out to the specified output. To get the "+
		"group file once the setup phase is done, you can run the `drand show "+
		"group` command")
	// new line
	fmt.Fprintln(output, "")
	groupP, shareErr := ctrlClient.InitDKGLeader(nodes, args.threshold, period,
		catchupPeriod, args.timeout, args.entropy, args.secret, offset, sch.ID, beaconID)

	if shareErr != nil {
		return fmt.Errorf("error setting up the network: %w", shareErr)
	}
	group, err := key.GroupFromProto(groupP)
	if err != nil {
		return fmt.Errorf("error interpreting the group from protobuf: %w", err)
	}
	return groupOut(c, group)
}

func loadCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}

	beaconID := getBeaconID(c)
	_, err = client.LoadBeacon(beaconID)
	if err != nil {
		return fmt.Errorf("could not reload the beacon process [%s]: %w", beaconID, err)
	}

	fmt.Fprintf(output, "Beacon process [%s] was loaded on drand.\n", beaconID)
	return nil
}

func reshareCmd(c *cli.Context) error {
	if c.Bool(leaderFlag.Name) {
		return leadReshareCmd(c)
	}

	args, err := getShareArgs(c)
	if err != nil {
		return err
	}

	if c.IsSet(periodFlag.Name) {
		return fmt.Errorf("%s flag is not allowed on resharing", periodFlag.Name)
	}

	if !c.IsSet(connectFlag.Name) {
		return fmt.Errorf("need to the address of the coordinator to create the group file")
	}
	coordAddress := c.String(connectFlag.Name)
	connectPeer := net.CreatePeer(coordAddress, args.isTLS)

	ctrlClient, err := net.NewControlClient(args.conf.ControlPort())
	if err != nil {
		return fmt.Errorf("could not create client: %w", err)
	}

	beaconID := getBeaconID(c)

	// resharing case needs the previous group
	var oldPath string
	if c.IsSet(transitionFlag.Name) {
		// daemon will try to the load the one stored
		oldPath = ""
	} else if c.IsSet(oldGroupFlag.Name) {
		var oldGroup = new(key.Group)
		if err := key.Load(c.String(oldGroupFlag.Name), oldGroup); err != nil {
			return fmt.Errorf("could not load drand from path: %w", err)
		}

		oldPath = c.String(oldGroupFlag.Name)

		if c.IsSet(beaconIDFlag.Name) {
			return fmt.Errorf("beacon id flag is not required when using --%s", oldGroupFlag.Name)
		}

		beaconID = common.GetCanonicalBeaconID(oldGroup.ID)
	}

	fmt.Fprintf(output, "Participating to the resharing. Beacon ID: [%s] \n", beaconID)

	groupP, shareErr := ctrlClient.InitReshare(connectPeer, args.secret, oldPath, args.force, beaconID)
	if shareErr != nil {
		return fmt.Errorf("error setting up the network: %w", shareErr)
	}
	group, err := key.GroupFromProto(groupP)
	if err != nil {
		return fmt.Errorf("error interpreting the group from protobuf: %w", err)
	}
	return groupOut(c, group)
}

func leadReshareCmd(c *cli.Context) error {
	args, err := getShareArgs(c)
	if err != nil {
		return err
	}

	if c.IsSet(periodFlag.Name) {
		return fmt.Errorf("%s flag is not allowed on resharing", periodFlag.Name)
	}

	if !c.IsSet(thresholdFlag.Name) || !c.IsSet(shareNodeFlag.Name) {
		return fmt.Errorf("leader needs to specify --nodes and --threshold for sharing")
	}

	nodes := c.Int(shareNodeFlag.Name)

	ctrlClient, err := net.NewControlClient(args.conf.ControlPort())
	if err != nil {
		return fmt.Errorf("could not create client: %w", err)
	}

	beaconID := getBeaconID(c)

	// resharing case needs the previous group
	var oldPath string
	if c.IsSet(transitionFlag.Name) {
		// daemon will try to the load the one stored
		oldPath = ""
	} else if c.IsSet(oldGroupFlag.Name) {
		var oldGroup = new(key.Group)
		if err := key.Load(c.String(oldGroupFlag.Name), oldGroup); err != nil {
			return fmt.Errorf("could not load drand from path: %w", err)
		}
		oldPath = c.String(oldGroupFlag.Name)

		if c.IsSet(beaconIDFlag.Name) {
			return fmt.Errorf("beacon id flag is not required when using --%s", oldGroupFlag.Name)
		}

		beaconID = common.GetCanonicalBeaconID(oldGroup.ID)
	}

	offset := int(core.DefaultResharingOffset.Seconds())
	if c.IsSet(beaconOffset.Name) {
		offset = c.Int(beaconOffset.Name)
	}
	catchupPeriod := time.Duration(-1)
	if c.IsSet(catchupPeriodFlag.Name) {
		catchupPeriodStr := c.String(catchupPeriodFlag.Name)
		if catchupPeriod, err = time.ParseDuration(catchupPeriodStr); err != nil {
			return fmt.Errorf("catchup period given is invalid: %w", err)
		}
	}

	fmt.Fprintf(output, "Initiating the resharing as a leader. Beacon ID: [%s] \n", beaconID)
	groupP, shareErr := ctrlClient.InitReshareLeader(nodes, args.threshold, args.timeout,
		catchupPeriod, args.secret, oldPath, offset, beaconID)

	if shareErr != nil {
		return fmt.Errorf("error setting up the network: %w", shareErr)
	}
	group, err := key.GroupFromProto(groupP)
	if err != nil {
		return fmt.Errorf("error interpreting the group from protobuf: %w", err)
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
	beaconID := getBeaconID(c)

	addresses := make([]*drand.Address, len(ips))
	for i := 0; i < len(ips); i++ {
		addresses[i] = &drand.Address{
			Address: ips[i],
			Tls:     isTLS,
		}
	}

	resp, err := client.RemoteStatus(c.Context, addresses, beaconID)
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
			return fmt.Errorf("cannot marshal the response ... %w", err)
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
		return fmt.Errorf("drand: can't ping the daemon ... %w", err)
	}
	fmt.Fprintf(output, "drand daemon is alive on port %s\n", controlPort(c))
	return nil
}

func remotePingToNode(addr string, tls bool) error {
	peer := net.CreatePeer(addr, tls)
	client := net.NewGrpcClient()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := client.Home(ctx, peer, &drand.HomeRequest{})
	if err != nil {
		return err
	}

	return nil
}

//nolint:gocyclo
func statusCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}

	listIds := c.IsSet(listIdsFlag.Name)
	allIds := c.IsSet(allBeaconsFlag.Name)
	beaconID := c.IsSet(beaconIDFlag.Name)

	if beaconID && (allIds || listIds) {
		return fmt.Errorf("drand: can't use --%s with --%s or --%s flags at the same time",
			beaconIDFlag.Name, allBeaconsFlag.Name, listIdsFlag.Name)
	}

	beaconIDsList := &control.ListBeaconIDsResponse{}
	if allIds || listIds {
		beaconIDsList, err = client.ListBeaconIDs()
		if err != nil {
			return fmt.Errorf("drand: can't get the list of running beacon ids on the daemon ... %w", err)
		}
	} else {
		beaconIDsList.Ids = append(beaconIDsList.Ids, getBeaconID(c))
	}

	if listIds {
		if c.IsSet(jsonFlag.Name) {
			str, err := json.Marshal(beaconIDsList)
			if err != nil {
				return fmt.Errorf("cannot marshal the response ... %w", err)
			}
			fmt.Fprintf(output, "%s \n", string(str))
			return nil
		}

		fmt.Fprintf(output, "running beacon ids on the node: [%s]\n", strings.Join(beaconIDsList.Ids, ", "))
		return nil
	}

	statuses := beaconIDsStatuses{Beacons: make(map[string]*control.StatusResponse)}
	for _, id := range beaconIDsList.Ids {
		resp, err := client.Status(id)
		if err != nil {
			return fmt.Errorf("drand: can't get the status of the network with id [%s]... %w", id, err)
		}

		if c.IsSet(jsonFlag.Name) {
			statuses.Beacons[id] = resp
			continue
		}

		fmt.Fprintf(output, "the status of network with id [%s] is: \n", id)
		fmt.Fprintf(output, "%s \n", core.StatusResponseToString(resp))
	}

	if c.IsSet(jsonFlag.Name) {
		str, err := json.Marshal(statuses)
		if err != nil {
			return fmt.Errorf("cannot marshal the response ... %w", err)
		}
		fmt.Fprintf(output, "%s \n", string(str))
	}

	return nil
}

func migrateCmd(c *cli.Context) error {
	conf := contextToConfig(c)

	if err := migration.MigrateSBFolderStructure(conf.ConfigFolder()); err != nil {
		return fmt.Errorf("cannot migrate folder structure, please try again. err: %w", err)
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
		return fmt.Errorf("drand: can't get the list of scheme ids availables ... %w", err)
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

	beaconID := getBeaconID(c)
	r, err := client.GroupFile(beaconID)
	if err != nil {
		return fmt.Errorf("fetching group file error: %w", err)
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

	beaconID := getBeaconID(c)
	resp, err := client.ChainInfo(beaconID)
	if err != nil {
		return fmt.Errorf("could not request chain info: %w", err)
	}

	ci, err := chain.InfoFromProto(resp)
	if err != nil {
		return fmt.Errorf("could not get correct chain info: %w", err)
	}

	return printChainInfo(c, ci)
}

func showPrivateCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}

	beaconID := getBeaconID(c)
	resp, err := client.PrivateKey(beaconID)
	if err != nil {
		return fmt.Errorf("could not request drand.private: %w", err)
	}

	return printJSON(resp)
}

func showPublicCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}

	beaconID := getBeaconID(c)
	resp, err := client.PublicKey(beaconID)
	if err != nil {
		return fmt.Errorf("drand: could not request drand.public: %w", err)
	}

	return printJSON(resp)
}

func showShareCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}

	beaconID := getBeaconID(c)
	resp, err := client.Share(beaconID)
	if err != nil {
		return fmt.Errorf("could not request drand.share: %w", err)
	}
	return printJSON(resp)
}

func backupDBCmd(c *cli.Context) error {
	client, err := controlClient(c)
	if err != nil {
		return err
	}

	outDir := c.String(outFlag.Name)
	beaconID := getBeaconID(c)
	err = client.BackupDB(outDir, beaconID)
	if err != nil {
		return fmt.Errorf("could not back up: %w", err)
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
		return nil, fmt.Errorf("can't instantiate control client: %w", err)
	}
	return client, nil
}

func printJSON(j interface{}) error {
	buff, err := json.MarshalIndent(j, "", "    ")
	if err != nil {
		return fmt.Errorf("could not JSON marshal: %w", err)
	}
	fmt.Fprintln(output, string(buff))
	return nil
}

func entropyInfoFromReader(c *cli.Context) (*control.EntropyInfo, error) {
	if c.IsSet(sourceFlag.Name) {
		_, err := os.Lstat(c.String(sourceFlag.Name))
		if err != nil {
			return nil, fmt.Errorf("cannot use given entropy source: %w", err)
		}
		source := c.String(sourceFlag.Name)
		ei := &control.EntropyInfo{
			Script:   source,
			UserOnly: c.Bool(userEntropyOnlyFlag.Name),
		}
		return ei, nil
	}
	//nolint
	return nil, nil
}

func selfSign(c *cli.Context) error {
	conf := contextToConfig(c)

	beaconID := getBeaconID(c)

	fs := key.NewFileStore(conf.ConfigFolderMB(), beaconID)
	pair, err := fs.LoadKeyPair()

	if err != nil {
		return fmt.Errorf("beacon id [%s] - loading private/public: %w", beaconID, err)
	}
	if pair.Public.ValidSignature() == nil {
		fmt.Fprintf(output, "beacon id [%s] - public identity already self signed.\n", beaconID)
		return nil
	}

	pair.SelfSign()
	if err := fs.SaveKeyPair(pair); err != nil {
		return fmt.Errorf("beacon id [%s] - saving identity: %w", beaconID, err)
	}

	fmt.Fprintf(output, "beacon id [%s] - Public identity self signed", beaconID)
	fmt.Fprintln(output, printJSON(pair.Public.TOML()))
	return nil
}

const refreshRate = 1000 * time.Millisecond

func syncCmd(c *cli.Context) error {
	ctrlClient, err := controlClient(c)
	if err != nil {
		return fmt.Errorf("unable to create control client: %w", err)
	}

	addrs := strings.Split(c.String(syncNodeFlag.Name), ",")
	channel, errCh, err := ctrlClient.StartSyncChain(
		c.Context,
		c.String(hashInfoReq.Name),
		addrs,
		!c.Bool(insecureFlag.Name),
		uint64(c.Int(upToFlag.Name)),
		getBeaconID(c))

	if err != nil {
		return fmt.Errorf("error asking to follow chain: %w", err)
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
	s.FinalMSG = "\nSync stopped\n"
	s.Start()
	defer s.Stop()
	for {
		select {
		case progress := <-channel:
			atomic.StoreUint64(&current, progress.Current)
			atomic.StoreUint64(&target, progress.Target)
		case err := <-errCh:
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("errror on following the chain: %w", err)
		}
	}
}

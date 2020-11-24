// Package drand is a distributed randomness beacon. It provides periodically an
// unpredictable, bias-resistant, and verifiable random value.
package drand

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	gonet "net"

	"github.com/BurntSushi/toml"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/core"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/urfave/cli/v2"
)

// default output of the drand operational commands
// the drand daemon use its own logging mechanism.
var output io.Writer = os.Stdout

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.version=`git describe --tags`
//   -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	version   = "master"
	gitCommit = "none"
	buildDate = "unknown"
)

const defaultPort = "8080"

func banner() {
	fmt.Fprintf(output, "drand %v (date %v, commit %v) by nikkolasg\n", version, buildDate, gitCommit)
}

var folderFlag = &cli.StringFlag{
	Name:  "folder",
	Value: core.DefaultConfigFolder(),
	Usage: "Folder to keep all drand cryptographic information, with absolute path.",
}

var verboseFlag = &cli.BoolFlag{
	Name:  "verbose",
	Usage: "If set, verbosity is at the debug level",
}

var tlsCertFlag = &cli.StringFlag{
	Name: "tls-cert",
	Usage: "Set the TLS certificate chain (in PEM format) for this drand node. " +
		"The certificates have to be specified as a list of whitespace-separated file paths. " +
		"This parameter is required by default and can only be omitted if the --tls-disable flag is used.",
}

var tlsKeyFlag = &cli.StringFlag{
	Name: "tls-key",
	Usage: "Set the TLS private key (in PEM format) for this drand node. " +
		"The key has to be specified as a file path. " +
		"This parameter is required by default and can only be omitted if the --tls-disable flag is used.",
}

var insecureFlag = &cli.BoolFlag{
	Name:  "tls-disable",
	Usage: "Disable TLS for all communications (not recommended).",
}

var controlFlag = &cli.StringFlag{
	Name:  "control",
	Usage: "Set the port you want to listen to for control port commands. If not specified, we will use the default port 8888.",
}

var metricsFlag = &cli.StringFlag{
	Name:  "metrics",
	Usage: "Launch a metrics server at the specified (host:)port.",
}

var privListenFlag = &cli.StringFlag{
	Name:  "private-listen",
	Usage: "Set the listening (binding) address of the private API. Useful if you have some kind of proxy.",
}

var pubListenFlag = &cli.StringFlag{
	Name:  "public-listen",
	Usage: "Set the listening (binding) address of the public API. Useful if you have some kind of proxy.",
}

var nodeFlag = &cli.StringFlag{
	Name:  "nodes",
	Usage: "Contact the nodes at the given list of whitespace-separated addresses which have to be present in group.toml.",
}

var roundFlag = &cli.IntFlag{
	Name: "round",
	Usage: "Request the public randomness generated at round num. If the drand beacon does not have the requested value," +
		" it returns an error. If not specified, the current randomness is returned.",
}

var certsDirFlag = &cli.StringFlag{
	Name:  "certs-dir",
	Usage: "directory containing trusted certificates (PEM format). Useful for testing and self signed certificates",
}

var outFlag = &cli.StringFlag{
	Name:  "out",
	Usage: "save the group file into a separate file instead of stdout",
}

var periodFlag = &cli.StringFlag{
	Name:  "period",
	Usage: "period to set when doing a setup",
}

var catchupPeriodFlag = &cli.StringFlag{
	Name:  "catchup-period",
	Usage: "Minimum period while in catchup. Set only by the leader of share / reshares",
	Value: "0s",
}

var thresholdFlag = &cli.IntFlag{
	Name:  "threshold",
	Usage: "threshold to use for the DKG",
}

var shareNodeFlag = &cli.IntFlag{
	Name:  "nodes",
	Usage: "number of nodes expected",
}

var transitionFlag = &cli.BoolFlag{
	Name: "transition",
	Usage: "When set, this flag indicates the share operation is a resharing. " +
		"The node will use the currently stored group as the basis for the resharing",
}

var forceFlag = &cli.BoolFlag{
	Name:  "force, f",
	Usage: "When set, this flag forces the daemon to start a new reshare operation." + "By default, it does not allow to restart one",
}

// secret flag is the "manual" security when the "leader"/coordinator creates the
// group: every participant must know this secret. It is not a consensus, not
// perfect, but since all members are known after the protocol, and members can
// decide to redo the setup, it works in practice well enough.
// XXX Add a manual check when the group is created so the user manually ACK.
var secretFlag = &cli.StringFlag{
	Name: "secret-file",
	Usage: "Specify the secret to use when doing the share so the leader knows you are an eligible potential participant." +
		" must be at least 32 characters.",
}

var connectFlag = &cli.StringFlag{
	Name:  "connect",
	Usage: "Address of the coordinator that will assemble the public keys and start the DKG",
}

var leaderFlag = &cli.BoolFlag{
	Name:  "leader",
	Usage: "Specify if this node should act as the leader for setting up the group",
}

var beaconOffset = &cli.IntFlag{
	Name: "beacon-delay",
	Usage: "Leader uses this flag to specify the genesis time or transition time as a delay from when " +
		" group is ready to run the share protocol",
}

var oldGroupFlag = &cli.StringFlag{
	Name: "from",
	Usage: "Old group.toml path to specify when a new node wishes to participate " +
		"in a resharing protocol. This flag is optional in case a node is already" +
		"included in the current DKG.",
}

var skipValidationFlag = &cli.BoolFlag{
	Name:  "skipValidation",
	Usage: "skips bls verification of beacon rounds for faster catchup.",
}

var timeoutFlag = &cli.StringFlag{
	Name:  "timeout",
	Usage: fmt.Sprintf("Timeout to use during the DKG, in string format. Default is %s", core.DefaultDKGTimeout),
}

var pushFlag = &cli.BoolFlag{
	Name: "push",
	Usage: "Push mode forces the daemon to start making beacon requests to the other node, " +
		"instead of waiting the other nodes contact it to catch-up on the round",
}

var sourceFlag = &cli.StringFlag{
	Name:  "source",
	Usage: "Source flag allows to provide an executable which output will be used as additional entropy during resharing step.",
}

var userEntropyOnlyFlag = &cli.BoolFlag{
	Name: "user-source-only",
	Usage: "user-source-only flag used with the source flag allows to only use the user's entropy to pick the dkg secret " +
		"(won't be mixed with crypto/rand). Should be used for reproducibility and debbuging purposes.",
}

var groupFlag = &cli.StringFlag{
	Name:  "group",
	Usage: "Test connections to nodes listed in the group",
}

var enablePrivateRand = &cli.BoolFlag{
	Name:  "private-rand",
	Usage: "Enables the private randomness feature on the daemon. By default, this feature is disabled.",
}

var hashOnly = &cli.BoolFlag{
	Name:  "hash",
	Usage: "Only print the hash of the group file",
}

var hashInfoFlag = &cli.StringFlag{
	Name:     "chain-hash",
	Usage:    "The hash of the chain info",
	Required: true,
}

// using a simple string flag because the StringSliceFlag is not intuitive
// see https://github.com/urfave/cli/issues/62
var syncNodeFlag = &cli.StringFlag{
	Name:     "sync-nodes",
	Usage:    "<ADDRESS:PORT>,<...> of (multiple) reachable drand daemon(s)",
	Required: true,
}

var upToFlag = &cli.IntFlag{
	Name:  "up-to",
	Usage: "Specify a round to which the drand daemon will stop following the chain",
	Value: 0,
}

var appCommands = []*cli.Command{
	{
		Name:  "start",
		Usage: "Start the drand daemon.",
		Flags: toArray(folderFlag, tlsCertFlag, tlsKeyFlag,
			insecureFlag, controlFlag, privListenFlag, pubListenFlag, metricsFlag,
			certsDirFlag, pushFlag, verboseFlag, enablePrivateRand, oldGroupFlag, skipValidationFlag),
		Action: func(c *cli.Context) error {
			banner()
			return startCmd(c)
		},
	},
	{
		Name:  "stop",
		Usage: "Stop the drand daemon.\n",
		Flags: toArray(controlFlag),
		Action: func(c *cli.Context) error {
			banner()
			return stopDaemon(c)
		},
	},
	{
		Name:  "share",
		Usage: "Launch a sharing protocol.",
		Flags: toArray(insecureFlag, controlFlag, oldGroupFlag,
			timeoutFlag, sourceFlag, userEntropyOnlyFlag, secretFlag,
			periodFlag, shareNodeFlag, thresholdFlag, connectFlag, outFlag,
			leaderFlag, beaconOffset, transitionFlag, forceFlag, catchupPeriodFlag),
		Action: func(c *cli.Context) error {
			banner()
			return shareCmd(c)
		},
	},
	{
		Name:  "follow",
		Usage: "follow and store a randomness chain",
		Flags: toArray(folderFlag, controlFlag, hashInfoFlag, syncNodeFlag,
			tlsCertFlag, insecureFlag, upToFlag),
		Action: followCmd,
	},
	{
		Name: "generate-keypair",
		Usage: "Generate the longterm keypair (drand.private, drand.public)" +
			"for this node.\n",
		ArgsUsage: "<address> is the address other nodes will be able to contact this node on (specified as 'private-listen' to the daemon)",
		Flags:     toArray(folderFlag, insecureFlag),
		Action: func(c *cli.Context) error {
			banner()
			return keygenCmd(c)
		},
	},

	{
		Name: "get",
		Usage: "get allows for public information retrieval from a remote " +
			"drand node.\n",
		Subcommands: []*cli.Command{
			{
				Name: "private",
				Usage: "Get private randomness from the drand beacon as " +
					"specified in group.toml. Only one node is contacted by " +
					"default. Requests are ECIES-encrypted towards the public " +
					"key of the contacted node. This command attempts to connect " +
					"to the drand beacon via TLS and falls back to " +
					"plaintext communication if the contacted node has not " +
					"activated TLS in which case it prints a warning.\n",
				ArgsUsage: "<group.toml> provides the group informations of " +
					"the nodes that we are trying to contact.",
				Flags:  toArray(insecureFlag, tlsCertFlag, nodeFlag),
				Action: getPrivateCmd,
			},
			{
				Name: "public",
				Usage: "Get the latest public randomness from the drand " +
					"beacon and verify it against the collective public key " +
					"as specified in group.toml. Only one node is contacted by " +
					"default. This command attempts to connect to the drand " +
					"beacon via TLS and falls back to plaintext communication " +
					"if the contacted node has not activated TLS in which case " +
					"it prints a warning.\n",
				Flags:  toArray(tlsCertFlag, insecureFlag, roundFlag, nodeFlag),
				Action: getPublicRandomness,
			},
			{
				Name:      "chain-info",
				Usage:     "Get the binding chain information that this nodes participates to",
				ArgsUsage: "`ADDRESS1` `ADDRESS2` ... provides the addresses of the node to try to contact to.",
				Flags:     toArray(tlsCertFlag, insecureFlag, hashOnly),
				Action:    getChainInfo,
			},
		},
	},
	{
		Name:  "util",
		Usage: "Multiple commands of utility functions, such as reseting a state, checking the connection of a peer...",
		Subcommands: []*cli.Command{
			{
				Name: "check",
				Usage: "Check node at the given `ADDRESS` (you can put multiple ones)" +
					" in the group for accessibility over the gRPC communication. If the node " +
					" is not running behind TLS, you need to pass the tls-disable flag. You can " +
					"also check a whole group's connectivity with the group flag.",
				Flags:  toArray(groupFlag, certsDirFlag, insecureFlag, verboseFlag),
				Action: checkConnection,
			},
			{
				Name:   "ping",
				Usage:  "pings the daemon checking its state\n",
				Flags:  toArray(controlFlag),
				Action: pingpongCmd,
			},
			{
				Name:   "reset",
				Usage:  "Resets the local distributed information (share, group file and random beacons). It KEEPS the private/public key pair.",
				Flags:  toArray(folderFlag, controlFlag),
				Action: resetCmd,
			},
			{
				Name: "del-beacon",
				Usage: "Delete all beacons from the given `ROUND` number until the head of the chain. " +
					" You MUST restart the daemon after that command.",
				Flags:  toArray(folderFlag),
				Action: deleteBeaconCmd,
			},
			{
				Name:   "self-sign",
				Usage:  "Signs the public identity of this node. Needed for backward compatibility with previous versions.",
				Flags:  toArray(folderFlag),
				Action: selfSign,
			},
		},
	},
	{
		Name: "show",
		Usage: "local information retrieval about the node's cryptographic " +
			"material. Show prints the information about the collective " +
			"public key (drand.cokey), the group details (group.toml), the " +
			"long-term private key (drand.private), the long-term public key " +
			"(drand.public), or the private key share (drand.share), " +
			"respectively.\n",
		Flags: toArray(folderFlag, controlFlag),
		Subcommands: []*cli.Command{
			{
				Name:   "share",
				Usage:  "shows the private share\n",
				Flags:  toArray(controlFlag),
				Action: showShareCmd,
			},
			{
				Name: "group",
				Usage: "shows the current group.toml used. The group.toml " +
					"may contain the distributed public key if the DKG has been " +
					"ran already.\n",
				Flags:  toArray(outFlag, controlFlag, hashOnly),
				Action: showGroupCmd,
			},
			{
				Name:   "chain-info",
				Usage:  "shows the chain information this node is participating to",
				Flags:  toArray(controlFlag, hashOnly),
				Action: showChainInfo,
			},
			{
				Name:   "private",
				Usage:  "shows the long-term private key of a node.\n",
				Flags:  toArray(controlFlag),
				Action: showPrivateCmd,
			},
			{
				Name:   "public",
				Usage:  "shows the long-term public key of a node.\n",
				Flags:  toArray(controlFlag),
				Action: showPublicCmd,
			},
		},
	},
}

// CLI runs the drand app
func CLI() *cli.App {
	app := cli.NewApp()
	app.Name = "drand"
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Fprintf(output, "drand %v (date %v, commit %v) by nikkolasg\n", version, buildDate, gitCommit)
	}

	app.ExitErrHandler = func(context *cli.Context, err error) {
		// override to prevent default behavior of calling OS.exit(1),
		// when tests expect to be able to run multiple commands.
	}
	app.Version = version
	app.Usage = "distributed randomness service"
	// =====Commands=====
	app.Commands = appCommands
	app.Flags = toArray(verboseFlag, folderFlag)
	app.Before = testWindows
	return app
}

func resetCmd(c *cli.Context) error {
	conf := contextToConfig(c)
	fmt.Fprintf(output, "You are about to delete your local share, group file and generated random beacons. "+
		"Are you sure you wish to perform this operation? [y/N]")
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading: %s", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" {
		fmt.Fprintf(output, "drand: not reseting the state.")
		return nil
	}
	store := key.NewFileStore(conf.ConfigFolder())
	if err := store.Reset(); err != nil {
		fmt.Fprintf(output, "drand: err reseting key store: %v\n", err)
		os.Exit(1)
	}
	if err := os.RemoveAll(conf.DBFolder()); err != nil {
		fmt.Fprintf(output, "drand: err reseting beacons database: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("drand: database reset")
	return nil
}

func askPort() string {
	for {
		var port string
		fmt.Fprintf(output, "No valid port given. Please, choose a port number (or ENTER for default port 8080): ")
		if _, err := fmt.Scanf("%s\n", &port); err != nil {
			continue
		}
		if port == "" {
			return defaultPort
		}
		_, err := strconv.Atoi(port)
		if len(port) > 2 && len(port) < 5 && err == nil {
			return port
		}
		return askPort()
	}
}

func testWindows(c *cli.Context) error {
	// x509 not available on windows: must run without TLS
	if runtime.GOOS == "windows" && !c.Bool("tls-disable") {
		return errors.New("TLS is not available on Windows, please disable TLS")
	}
	return nil
}

func keygenCmd(c *cli.Context) error {
	args := c.Args()
	if !args.Present() {
		return errors.New("missing drand address in argument. Abort")
	}
	addr := args.First()
	var validID = regexp.MustCompile(`:\d+$`)
	if !validID.MatchString(addr) {
		fmt.Println("Invalid port.")
		addr = addr + ":" + askPort()
	}
	var priv *key.Pair
	if c.Bool(insecureFlag.Name) {
		fmt.Println("Generating private / public key pair without TLS.")
		priv = key.NewKeyPair(addr)
	} else {
		fmt.Println("Generating private / public key pair with TLS indication")
		priv = key.NewTLSKeyPair(addr)
	}

	config := contextToConfig(c)
	fileStore := key.NewFileStore(config.ConfigFolder())

	if _, err := fileStore.LoadKeyPair(); err == nil {
		fmt.Fprintf(output, "Keypair already present in `%s`.\nRemove them before generating new one\n", config.ConfigFolder())
		return nil
	}
	if err := fileStore.SaveKeyPair(priv); err != nil {
		return fmt.Errorf("could not save key: %s", err)
	}
	fullpath := path.Join(config.ConfigFolder(), key.KeyFolderName)
	absPath, err := filepath.Abs(fullpath)
	if err != nil {
		return fmt.Errorf("err getting full path: %s", err)
	}
	fmt.Println("Generated keys at ", absPath)
	var buff bytes.Buffer
	if err := toml.NewEncoder(&buff).Encode(priv.Public.TOML()); err != nil {
		panic(err)
	}
	buff.WriteString("\n")
	fmt.Println(buff.String())
	return nil
}

func groupOut(c *cli.Context, group *key.Group) error {
	if c.IsSet("out") {
		groupPath := c.String("out")
		if err := key.Save(groupPath, group, false); err != nil {
			return fmt.Errorf("drand: can't save group to specified file name: %v", err)
		}
	} else if c.Bool(hashOnly.Name) {
		fmt.Fprintf(output, "%x\n", group.Hash())
	} else {
		var buff bytes.Buffer
		if err := toml.NewEncoder(&buff).Encode(group.TOML()); err != nil {
			return fmt.Errorf("drand: can't encode group to TOML: %v", err)
		}
		buff.WriteString("\n")
		fmt.Fprintf(output, "Copy the following snippet into a new group.toml file\n")
		fmt.Fprint(output, buff.String())
		fmt.Fprintf(output, "\nHash of the group configuration: %x\n", group.Hash())
	}
	return nil
}

func getThreshold(c *cli.Context) (int, error) {
	var threshold = key.DefaultThreshold(c.NArg())
	if c.IsSet(thresholdFlag.Name) {
		var localThr = c.Int(thresholdFlag.Name)
		if localThr < threshold {
			return 0, fmt.Errorf("drand: threshold specified too low %d/%d", localThr, threshold)
		}
		return localThr, nil
	}
	return threshold, nil
}

func checkConnection(c *cli.Context) error {
	var names []string
	if c.IsSet(groupFlag.Name) {
		if err := testEmptyGroup(c.String(groupFlag.Name)); err != nil {
			return err
		}
		group := new(key.Group)
		if err := key.Load(c.String(groupFlag.Name), group); err != nil {
			return fmt.Errorf("loading group failed: %s", err)
		}
		for _, id := range group.Nodes {
			names = append(names, id.Address())
		}
	} else if c.Args().Present() {
		for _, serverAddr := range c.Args().Slice() {
			_, _, err := gonet.SplitHostPort(serverAddr)
			if err != nil {
				return fmt.Errorf("error for address %s: %s", serverAddr, err)
			}
			names = append(names, serverAddr)
		}
	} else {
		return fmt.Errorf("drand: check-group expects a list of identities or %s flag", groupFlag.Name)
	}
	conf := contextToConfig(c)

	var isVerbose = c.IsSet(verboseFlag.Name)
	var allGood = true
	var invalidIds []string
	for _, address := range names {
		err := checkIdentityAddress(conf, address, !c.Bool(insecureFlag.Name))
		if err != nil {
			if isVerbose {
				fmt.Fprintf(output, "drand: error checking id %s: %s\n", address, err)
			} else {
				fmt.Fprintf(output, "drand: error checking id %s\n", address)
			}
			allGood = false
			invalidIds = append(invalidIds, address)
			continue
		}
		fmt.Fprintf(output, "drand: id %s answers correctly\n", address)
	}
	if !allGood {
		return fmt.Errorf("following nodes don't answer: %s", strings.Join(invalidIds, ","))
	}
	return nil
}

func checkIdentityAddress(conf *core.Config, addr string, tls bool) error {
	peer := net.CreatePeer(addr, tls)
	client := net.NewGrpcClientFromCertManager(conf.Certs())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	identity, err := client.GetIdentity(ctx, peer, &drand.IdentityRequest{})
	if err != nil {
		return err
	}
	id, err := key.IdentityFromProto(identity)
	if err != nil {
		return err
	}
	if id.Address() != addr {
		return fmt.Errorf("mismatch of address: contact %s reply with %s", addr, id.Address())
	}
	return nil
}

// deleteBeaconCmd deletes all beacon in the database from the given round until
// the head of the chain
func deleteBeaconCmd(c *cli.Context) error {
	conf := contextToConfig(c)
	startRoundStr := c.Args().First()
	sr, err := strconv.Atoi(startRoundStr)
	if err != nil {
		return fmt.Errorf("given round not valid: %d", sr)
	}
	startRound := uint64(sr)
	store, err := boltdb.NewBoltStore(conf.DBFolder(), conf.BoltOptions())
	if err != nil {
		return fmt.Errorf("invalid bolt store creation: %s", err)
	}
	defer store.Close()
	lastBeacon, err := store.Last()
	if err != nil {
		return fmt.Errorf("can't fetch last beacon: %s", err)
	}
	if startRound > lastBeacon.Round {
		return fmt.Errorf("given round is ahead of the chain: %d", lastBeacon.Round)
	}
	if c.IsSet(verboseFlag.Name) {
		fmt.Println("Planning to delete ", lastBeacon.Round-startRound, " beacons")
	}
	for round := startRound; round <= lastBeacon.Round; round++ {
		err := store.Del(round)
		if err != nil {
			return fmt.Errorf("error deleting round %d: %s", round, err)
		}
		if c.IsSet(verboseFlag.Name) {
			fmt.Println("- Deleted beacon round ", round)
		}
	}
	return nil
}

func toArray(flags ...cli.Flag) []cli.Flag {
	return flags
}

func getGroup(c *cli.Context) (*key.Group, error) {
	g := &key.Group{}
	groupPath := c.Args().First()
	if err := testEmptyGroup(groupPath); err != nil {
		return nil, err
	}
	if err := key.Load(groupPath, g); err != nil {
		return nil, fmt.Errorf("drand: error loading group file: %s", err)
	}
	return g, nil
}

func contextToConfig(c *cli.Context) *core.Config {
	var opts []core.ConfigOption

	if c.IsSet(verboseFlag.Name) {
		opts = append(opts, core.WithLogLevel(log.LogDebug))
	} else {
		opts = append(opts, core.WithLogLevel(log.LogInfo))
	}

	if c.IsSet(pubListenFlag.Name) {
		opts = append(opts, core.WithPublicListenAddress(c.String(pubListenFlag.Name)))
	}
	if c.IsSet(privListenFlag.Name) {
		opts = append(opts, core.WithPrivateListenAddress(c.String(privListenFlag.Name)))
	}

	port := c.String(controlFlag.Name)
	if port != "" {
		opts = append(opts, core.WithControlPort(port))
	}
	if c.IsSet(folderFlag.Name) {
		opts = append(opts, core.WithConfigFolder(c.String(folderFlag.Name)))
	}
	opts = append(opts, core.WithVersion(fmt.Sprintf("drand/%s (%s)", version, gitCommit)))

	if c.Bool("tls-disable") {
		opts = append(opts, core.WithInsecure())
		if c.IsSet("tls-cert") || c.IsSet("tls-key") {
			panic("option 'tls-disable' used with 'tls-cert' or 'tls-key': combination is not valid")
		}
	} else {
		certPath, keyPath := c.String("tls-cert"), c.String("tls-key")
		opts = append(opts, core.WithTLS(certPath, keyPath))
	}
	if c.IsSet("certs-dir") {
		paths, err := fs.Files(c.String("certs-dir"))
		if err != nil {
			panic(err)
		}
		opts = append(opts, core.WithTrustedCerts(paths...))
	}
	if c.Bool(enablePrivateRand.Name) {
		opts = append(opts, core.WithPrivateRandomness())
	}
	conf := core.NewConfig(opts...)
	return conf
}

func getNodes(c *cli.Context) ([]*key.Node, error) {
	group, err := getGroup(c)
	if err != nil {
		return nil, err
	}
	var ids []*key.Node
	gids := group.Nodes
	if c.IsSet("nodes") {
		// search nodes listed on the flag in the group
		for _, addr := range strings.Split(c.String("nodes"), ",") {
			for _, gid := range gids {
				if gid.Addr == addr {
					ids = append(ids, gid)
				}
			}
		}
		if len(ids) == 0 {
			return nil, errors.New("addresses specified don't exist in group.toml")
		}
	} else {
		// select them all in order
		ids = gids
	}
	if len(ids) == 0 {
		return nil, errors.New("no nodes specified with --nodes are in the group file")
	}
	return ids, nil
}

func testEmptyGroup(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("can't open group path: %v", err)
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil {
		return fmt.Errorf("can't open file info: %v", err)
	}
	if fi.Size() == 0 {
		return errors.New("group file empty")
	}
	return nil
}

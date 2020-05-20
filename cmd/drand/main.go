// drand is a distributed randomness beacon. It provides periodically an
// unpredictable, bias-resistant, and verifiable random value.
package drand

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	gonet "net"

	"github.com/BurntSushi/toml"
	"github.com/drand/drand/beacon"
	"github.com/drand/drand/core"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/nikkolasg/slog"
	"github.com/urfave/cli/v2"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	version   = "master"
	gitCommit = "none"
	buildDate = "unknown"
)

const gname = "group.toml"
const dpublic = "dist_key.public"
const defaultPort = "8080"

func banner() {
	fmt.Printf("drand %v (date %v, commit %v) by nikkolasg\n", version, buildDate, gitCommit)
	s := "WARNING: this software has NOT received a full audit and must be used with caution and probably NOT in a production environment.\n"
	fmt.Printf(s)
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
	Name:  "round",
	Usage: "Request the public randomness generated at round num. If the drand beacon does not have the requested value, it returns an error. If not specified, the current randomness is returned.",
}

var fromGroupFlag = &cli.StringFlag{
	Name:  "from",
	Usage: "If you want to replace keys into an existing group.toml file to perform a resharing later on, run the group command and specify the existing group.toml file with this flag.",
}

var certsDirFlag = &cli.StringFlag{
	Name:  "certs-dir",
	Usage: "directory containing trusted certificates. Useful for testing and self signed certificates",
}

var outFlag = &cli.StringFlag{
	Name:  "out",
	Usage: "save the group file into a separate file instead of stdout",
}

var periodFlag = &cli.StringFlag{
	Name:  "period",
	Usage: "period to set when doing a setup",
}

var thresholdFlag = &cli.IntFlag{
	Name:     "threshold",
	Required: true,
	Usage:    "threshold to use for the DKG",
}

var shareNodeFlag = &cli.IntFlag{
	Name:     "nodes",
	Required: true,
	Usage:    "number of nodes expected",
}

var transitionFlag = &cli.BoolFlag{
	Name:  "transition",
	Usage: "When set, this flag indicates the share operation is a resharing. The node will use the currently stored group as the basis for the resharing",
}

// secret flag is the "manual" security when the "leader"/coordinator creates the
// group: every participant must know this secret. It is not a consensus, not
// perfect, but since all members are known after the protocol, and members can
// decide to redo the setup, it works in practice well enough.
// XXX Add a manual check when the group is created so the user manually ACK.
var secretFlag = &cli.StringFlag{
	Name:     "secret",
	Required: true,
	Usage:    "Specify the secret to use when doing the share so the leader knows you are an eligible potential participant",
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
	Name:  "beacon-delay",
	Usage: "Leader uses this flag to specify the genesis time or transition time as a delay from when group is ready to run the share protocol",
}

// XXX deleted flags : debugFlag, outFlag, groupFlag, seedFlag, periodFlag, distKeyFlag, thresholdFlag.

var oldGroupFlag = &cli.StringFlag{
	Name: "from",
	Usage: "Old group.toml path to specify when a new node wishes to participate " +
		"in a resharing protocol. This flag is optional in case a node is already" +
		"included in the current DKG.",
}

var timeoutFlag = &cli.StringFlag{
	Name:  "timeout",
	Usage: fmt.Sprintf("Timeout to use during the DKG, in string format. Default is %s", core.DefaultDKGTimeout),
}

var pushFlag = &cli.BoolFlag{
	Name:  "push",
	Usage: "Push mode forces the daemon to start making beacon requests to the other node, instead of waiting the other nodes contact it to catch-up on the round",
}

var sourceFlag = &cli.StringFlag{
	Name:  "source",
	Usage: "Source flag allows to provide an executable which output will be used as additional entropy during resharing step.",
}

var userEntropyOnlyFlag = &cli.BoolFlag{
	Name:  "user-source-only",
	Usage: "user-source-only flag used with the source flag allows to only use the user's entropy to pick the dkg secret (won't be mixed with crypto/rand). Should be used for reproducibility and debbuging purposes.",
}

var startInFlag = &cli.StringFlag{
	Name:  "start-in",
	Usage: "Duration to parse in which the setup or resharing phase will start. This flags sets the genesis time  or transition time in 'start-in' period from now.",
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
	Name:  "hash-only",
	Usage: "Only print the hash of the group file",
}

func CLI() {
	app := cli.NewApp()

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("drand %v (date %v, commit %v) by nikkolasg\n", version, buildDate, gitCommit)
	}

	app.Version = version
	app.Usage = "distributed randomness service"
	// =====Commands=====
	app.Commands = []*cli.Command{
		&cli.Command{
			Name:  "start",
			Usage: "Start the drand daemon.",
			Flags: toArray(folderFlag, tlsCertFlag, tlsKeyFlag,
				insecureFlag, controlFlag, privListenFlag, pubListenFlag, metricsFlag,
				certsDirFlag, pushFlag, verboseFlag, enablePrivateRand),
			Action: func(c *cli.Context) error {
				banner()
				return startCmd(c)
			},
		},
		&cli.Command{
			Name:  "stop",
			Usage: "Stop the drand daemon.\n",
			Flags: toArray(controlFlag),
			Action: func(c *cli.Context) error {
				banner()
				return stopDaemon(c)
			},
		},
		&cli.Command{
			Name:  "share",
			Usage: "Launch a sharing protocol.",
			Flags: toArray(insecureFlag, controlFlag, oldGroupFlag,
				timeoutFlag, sourceFlag, userEntropyOnlyFlag, secretFlag,
				periodFlag, shareNodeFlag, thresholdFlag, connectFlag, outFlag,
				leaderFlag, beaconOffset, transitionFlag),
			Action: func(c *cli.Context) error {
				banner()
				return shareCmd(c)
			},
		},
		&cli.Command{
			Name: "generate-keypair",
			Usage: "Generate the longterm keypair (drand.private, drand.public)" +
				"for this node.\n",
			ArgsUsage: "<address> is the public address for other nodes to contact",
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
					Flags: toArray(insecureFlag, tlsCertFlag, nodeFlag),
					Action: func(c *cli.Context) error {
						return getPrivateCmd(c)
					},
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
					Flags: toArray(tlsCertFlag, insecureFlag, roundFlag, nodeFlag),
					Action: func(c *cli.Context) error {
						return getPublicRandomness(c)
					},
				},
				{
					Name: "cokey",
					Usage: "Get distributed public key generated during the " +
						"DKG step.",
					ArgsUsage: "`ADDRESS` provides the address of the node",
					Flags:     toArray(tlsCertFlag, insecureFlag),
					Action: func(c *cli.Context) error {
						return getCokeyCmd(c)
					},
				},
			},
		},
		{
			Name:  "util",
			Usage: "Multiple commands of utility functions, such as reseting a state, checking the connection of a peer...",
			Subcommands: []*cli.Command{
				&cli.Command{
					Name: "check",
					Usage: "Check node at the given `ADDRESS` (you can put multiple ones)" +
						" in the group for accessibility over the gRPC communication. If the node " +
						" is not running behind TLS, you need to pass the tls-disable flag. You can " +
						"also check a whole group's connectivity with the group flag.",
					Flags: toArray(groupFlag, certsDirFlag, insecureFlag),
					Action: func(c *cli.Context) error {
						banner()
						return checkConnection(c)
					},
				},
				{
					Name:  "ping",
					Usage: "pings the daemon checking its state\n",
					Flags: toArray(controlFlag),
					Action: func(c *cli.Context) error {
						return pingpongCmd(c)
					},
				},
				{
					Name:  "reset",
					Usage: "Resets the local distributed information (share, group file and random beacons). It KEEPS the private/public key pair.",
					Flags: toArray(folderFlag, controlFlag),
					Action: func(c *cli.Context) error {
						return resetCmd(c)
					},
				},
				{
					Name:  "del-beacon",
					Usage: "Delete all beacons from the given `ROUND` number until the head of the chain. You MUST restart the daemon after that command.",
					Flags: toArray(folderFlag),
					Action: func(c *cli.Context) error {
						return deleteBeaconCmd(c)
					},
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
					Name:  "share",
					Usage: "shows the private share\n",
					Flags: toArray(controlFlag),
					Action: func(c *cli.Context) error {
						return showShareCmd(c)
					},
				},
				{
					Name: "group",
					Usage: "shows the current group.toml used. The group.toml " +
						"may contain the distributed public key if the DKG has been " +
						"ran already.\n",
					Flags: toArray(outFlag, controlFlag, hashOnly),
					Action: func(c *cli.Context) error {
						return showGroupCmd(c)
					},
				},
				{
					Name:  "cokey",
					Usage: "shows the collective key generated during DKG.\n",
					Flags: toArray(controlFlag),
					Action: func(c *cli.Context) error {
						return showCokeyCmd(c)
					},
				},
				{
					Name:  "private",
					Usage: "shows the long-term private key of a node.\n",
					Flags: toArray(controlFlag),
					Action: func(c *cli.Context) error {
						return showPrivateCmd(c)
					},
				},
				{
					Name:  "public",
					Usage: "shows the long-term public key of a node.\n",
					Flags: toArray(controlFlag),
					Action: func(c *cli.Context) error {
						return showPublicCmd(c)
					},
				},
			},
		},
	}
	app.Flags = toArray(verboseFlag, folderFlag)
	app.Before = func(c *cli.Context) error {
		testWindows(c)
		return nil
	}
	if err := app.Run(os.Args); err != nil {
		slog.Fatalf("drand: error running app: %s", err)
	}
}

func resetCmd(c *cli.Context) error {
	conf := contextToConfig(c)
	fmt.Printf("You are about to delete your local share, group file and generated random beacons. Are you sure you wish to perform this operation? [y/N]")
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		slog.Fatal("error reading: ", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" {
		slog.Print("drand: not reseting the state.")
		return nil
	}
	store := key.NewFileStore(conf.ConfigFolder())
	if err := store.Reset(); err != nil {
		fmt.Printf("drand: err reseting key store: %v\n", err)
		os.Exit(1)
	}
	if err := os.RemoveAll(conf.DBFolder()); err != nil {
		fmt.Printf("drand: err reseting beacons database: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("drand: database reset")
	return nil
}

func resetBeaconDB(config *core.Config) bool {
	if _, err := os.Stat(config.DBFolder()); err == nil {
		fmt.Printf("INCONSISTENT STATE: A beacon database exists already.\n"+
			"drand support only one identity at the time and thus needs to delete "+
			"the existing beacon database.\nCurrent folder is %s.\nAccept to delete "+
			"database ? [Y/n]: ", config.DBFolder())
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			slog.Fatal("error reading: ", err)
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" {
			slog.Print("drand: not deleting the database.")
			return true
		}

		if err := os.RemoveAll(config.DBFolder()); err != nil {
			slog.Fatal(err)
		}
		slog.Print("drand: removed existing beacon database.")
	}
	return false
}

func askPort() string {
	for {
		var port string
		slog.Print("No valid port given. Please, choose a port number (or ENTER for default port 8080): ")
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

func testWindows(c *cli.Context) {
	//x509 not available on windows: must run without TLS
	if runtime.GOOS == "windows" && !c.Bool("tls-disable") {
		fatal("TLS is not available on Windows, please disable TLS")
	}
}

func fatal(str string, args ...interface{}) {
	fmt.Printf(str+"\n", args...)
	os.Exit(1)
}

func keygenCmd(c *cli.Context) error {
	args := c.Args()
	if !args.Present() {
		fatal("Missing drand address in argument. Abort.")
	}
	addr := args.First()
	var validID = regexp.MustCompile(`[:][0-9]+$`)
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
	fs := key.NewFileStore(config.ConfigFolder())

	if _, err := fs.LoadKeyPair(); err == nil {
		fmt.Printf("Keypair already present in `%s`.\nRemove them before generating new one\n", config.ConfigFolder())
		return nil
	}
	if err := fs.SaveKeyPair(priv); err != nil {
		fatal("could not save key: ", err)
	}
	fullpath := path.Join(config.ConfigFolder(), key.KeyFolderName)
	absPath, err := filepath.Abs(fullpath)
	if err != nil {
		fatal("err getting full path: ", err)
	}
	fmt.Println("Generated keys at ", absPath)
	fmt.Println("You can copy paste the following snippet to a common group.toml file:")
	var buff bytes.Buffer
	buff.WriteString("[[Nodes]]\n")
	if err := toml.NewEncoder(&buff).Encode(priv.Public.TOML()); err != nil {
		panic(err)
	}
	buff.WriteString("\n")
	fmt.Println(buff.String())
	fmt.Println("Or just collect all public key files and use the group command!")
	return nil
}

func groupOut(c *cli.Context, group *key.Group) {
	if c.IsSet("out") {
		groupPath := c.String("out")
		if err := key.Save(groupPath, group, false); err != nil {
			fatal("drand: can't save group to specified file name: %v", err)
		}
	} else if c.Bool(hashOnly.Name) {
		fmt.Printf("%x\n", group.Hash())
	} else {
		var buff bytes.Buffer
		if err := toml.NewEncoder(&buff).Encode(group.TOML()); err != nil {
			fatal("drand: can't encode group to TOML: %v", err)
		}
		buff.WriteString("\n")
		fmt.Printf("Copy the following snippet into a new group.toml file\n")
		fmt.Printf(buff.String())
		fmt.Printf("\nHash of the group configuration: %x\n", group.Hash())
	}
}

func getThreshold(c *cli.Context) int {
	var threshold = key.DefaultThreshold(c.NArg())
	if c.IsSet(thresholdFlag.Name) {
		var localThr = c.Int(thresholdFlag.Name)
		if localThr < threshold {
			fatal(fmt.Sprintf("drand: threshold specified too low %d/%d", localThr, threshold))
		}
		return localThr
	}
	return threshold
}

func getPublicKeys(c *cli.Context) []*key.Identity {
	publics := make([]*key.Identity, c.NArg())
	for i, str := range c.Args().Slice() {
		pub := &key.Identity{}
		fmt.Printf("drand: reading public identity from %s\n", str)
		if err := key.Load(str, pub); err != nil {
			fatal("drand: can't load key %d: %v", i, err)
		}
		publics[i] = pub
	}
	return publics
}

func checkConnection(c *cli.Context) error {
	var names []string
	if c.IsSet(groupFlag.Name) {
		testEmptyGroup(c.String(groupFlag.Name))
		group := new(key.Group)
		if err := key.Load(c.String(groupFlag.Name), group); err != nil {
			fatal("drand: loading group failed")
		}
		for _, id := range group.Nodes {
			names = append(names, id.Address())
		}
	} else if c.Args().Present() {
		for _, serverAddr := range c.Args().Slice() {
			_, _, err := gonet.SplitHostPort(serverAddr)
			if err != nil {
				fatal("error for address %s: %s", serverAddr, err)
			}
			names = append(names, serverAddr)
		}
	} else {
		fatal(fmt.Sprintf("drand: check-group expects a list of identities or %s flag", groupFlag.Name))
	}
	conf := contextToConfig(c)

	var isVerbose = c.IsSet(verboseFlag.Name)
	var allGood = true
	var invalidIds []string
	for _, address := range names {
		peer := net.CreatePeer(address, !c.Bool(insecureFlag.Name))
		client := net.NewGrpcClientFromCertManager(conf.Certs())
		_, err := client.Home(context.Background(), peer, &drand.HomeRequest{})
		if err != nil {
			if isVerbose {
				fmt.Printf("drand: error checking id %s: %s\n", peer.Address(), err)
			} else {
				fmt.Printf("drand: error checking id %s\n", peer.Address())
			}
			allGood = false
			invalidIds = append(invalidIds, peer.Address())
			continue
		}
		fmt.Printf("drand: id %s answers correctly\n", peer.Address())
	}
	if !allGood {
		return fmt.Errorf("Following nodes don't answer: %s", strings.Join(invalidIds, ","))
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
	store, err := beacon.NewBoltStore(conf.DBFolder(), conf.BoltOptions())
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
			return fmt.Errorf("Error deleting round %d: %s\n", round, err)
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

func getGroup(c *cli.Context) *key.Group {
	g := &key.Group{}
	groupPath := c.Args().First()
	testEmptyGroup(groupPath)
	if err := key.Load(groupPath, g); err != nil {
		fatal("drand: error loading group file: %s", err)
	}
	return g
}

// keyIDFromAddr looks at every node in the group file to retrieve to *key.Identity
func keyIDFromAddr(addr string, group *key.Group) *key.Identity {
	ids := group.Nodes
	for _, id := range ids {
		if id.Address() == addr {
			return id.Identity
		}
	}
	fatal("Could not retrive the node you are trying to contact in the group file.")
	return nil
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
	config := c.String(folderFlag.Name)
	opts = append(opts, core.WithConfigFolder(config))

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

func getNodes(c *cli.Context) []*key.Node {
	group := getGroup(c)
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
			fatal("drand: addresses specified don't exist in group.toml")
		}
	} else {
		// select them all in order
		ids = gids
	}
	if len(ids) == 0 {
		fatal("drand: no nodes specified with --nodes are in the group file")
	}
	return ids
}

func testEmptyGroup(path string) {
	file, err := os.Open(path)
	defer file.Close()
	if err != nil {
		fatal("drand: can't opern group path: %v", err)
	}
	fi, err := file.Stat()
	if err != nil {
		fatal("drand: can't open file info: %v", err)
	}
	if fi.Size() == 0 {
		fatal("drand: group file empty")
	}
}

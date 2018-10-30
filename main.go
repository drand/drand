// drand is a distributed randomness beacon. It provides periodically an
// unpredictable, bias-resistant, and verifiable random value.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/dedis/drand/core"
	"github.com/dedis/drand/fs"
	"github.com/dedis/drand/key"
	"github.com/nikkolasg/slog"
	"github.com/urfave/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const gname = "group.toml"
const dpublic = "dist_key.public"
const default_port = "8080"

func banner() {
	fmt.Printf("drand vtest-%s by nikkolasg @ DEDIS\n", version)
	s := "WARNING: this software has NOT received a full audit and must be \n" +
		"used with caution and probably NOT in a production environment.\n"
	fmt.Printf(s)
}

var folderFlag = cli.StringFlag{
	Name:  "folder, f",
	Value: core.DefaultConfigFolder(),
	Usage: "Folder to keep all drand cryptographic informations, in absolute form.",
}
var leaderFlag = cli.BoolFlag{
	Name:  "leader",
	Usage: "Set this node as the initator of the distributed key generation process.",
}
var verboseFlag = cli.IntFlag{
	Name:  "verbose, V",
	Value: 0,
	Usage: "Set verbosity to the given level. 0 for normal output, 1 for informational output and 2 for debug output.",
}

var tlsCertFlag = cli.StringFlag{
	Name: "tls-cert, c",
	Usage: "Set the TLS certificate chain (in PEM format) for this drand node. " +
		"The certificates have to be specified as a list of whitespace-separated file paths. " +
		"This parameter is required by default and can only be omitted if the --tls-disable flag is used.",
}

var tlsKeyFlag = cli.StringFlag{
	Name: "tls-key, k",
	Usage: "Set the TLS private key (in PEM format) for this drand node. " +
		"The keys have to be specified as a list of whitespace-separated file paths. " +
		"This parameter is required by default and can only be omitted if the --tls-disable flag is used.",
}

var insecureFlag = cli.BoolFlag{
	Name:  "tls-disable, d",
	Usage: "Disable TLS for all communications (not recommended).",
}

var controlFlag = cli.StringFlag{
	Name:  "control",
	Usage: "Set the port you want to listen to for control port commands. If not specified, we will use the default port 8888.",
}

var listenFlag = cli.StringFlag{
	Name:  "listen, l",
	Usage: "Set the listening (binding) address. Useful if you have some kind of proxy.",
}

var nodeFlag = cli.StringFlag{
	Name:  "nodes, n",
	Usage: "Contact the nodes at the given list of whitespace-separated addresses which have to be present in group.toml.",
}

var roundFlag = cli.IntFlag{
	Name:  "round, r",
	Usage: "Request the public randomness generated at round num. If the drand beacon does not have the requested value, it returns an error. If not specified, the current randomness is returned.",
}

var groupFlag = cli.StringFlag{
	Name:  "group, g",
	Usage: "If you want to merge keys into an existing group.toml file, run the group command and specify the group.toml file with this flag.",
}

var certsDirFlag = cli.StringFlag{
	Name:  "certs-dir",
	Usage: "directory containing trusted certificates. Useful for testing and self signed certificates",
}

var outFlag = cli.StringFlag{
	Name: "out, o",
	Usage: "indicates to save the requested information into a separate file" +
		" instead of stdout",
}

var periodFlag = cli.StringFlag{
	Name:  "period",
	Usage: "period to write in the group.toml file",
}

// XXX deleted flags : debugFlag, outFlag, groupFlag, seedFlag, periodFlag, distKeyFlag, thresholdFlag.

var oldGroupFlag = cli.StringFlag{
	Name: "from",
	Usage: "Old group.toml path to specify when a new node wishes to participate " +
		"in a resharing protocol. This flag is optional in case a node is already" +
		"included in the current DKG.",
}

func main() {
	app := cli.NewApp()
	app.Version = version
	app.Usage = "distributed randomness service"
	// =====Commands=====
	app.Commands = []cli.Command{
		cli.Command{
			Name:  "start",
			Usage: "Start the drand daemon.",
			Flags: toArray(folderFlag, tlsCertFlag, tlsKeyFlag,
				insecureFlag, controlFlag, listenFlag, certsDirFlag),
			Action: func(c *cli.Context) error {
				banner()
				return startCmd(c)
			},
		},
		cli.Command{
			Name:  "stop",
			Usage: "Stop the drand daemon.\n",
			Action: func(c *cli.Context) error {
				banner()
				return stopCmd(c)
			},
		},
		cli.Command{
			Name: "share",
			Usage: "Launch a sharing protocol. If one group is given as " +
				"argument, drand launch a DKG protocol to create a distributed " +
				"keypair between all participants listed in the group. A " +
				"existing group can also issue new shares to a new group: use " +
				"the flag --from to specify the the current group and give " +
				"the new group as argument. Specify the --leader flag to make " +
				"this daemon start the protocol\n",
			ArgsUsage: "<group.toml> group file",
			Flags: toArray(folderFlag, insecureFlag, controlFlag,
				leaderFlag, oldGroupFlag),
			Action: func(c *cli.Context) error {
				banner()
				return shareCmd(c)
			},
		},
		cli.Command{
			Name: "generate-keypair",
			Usage: "Generate the longterm keypair (drand.private, drand.public)" +
				"for this node.\n",
			ArgsUsage: "<address> is the public address for other nodes to contact",
			Flags:     toArray(insecureFlag),
			Action: func(c *cli.Context) error {
				banner()
				return keygenCmd(c)
			},
		},
		cli.Command{
			Name: "group",
			Usage: "Merge the given list of whitespace-separated drand.public " +
				"keys into the group.toml file if one is provided, if not create " +
				"a new group.toml file with the given identites.\n",
			ArgsUsage: "<key1 key2 key3...> must be the identities of the group " +
				"to create/to insert into the group",
			Flags: toArray(groupFlag, outFlag, periodFlag),
			Action: func(c *cli.Context) error {
				banner()
				return groupCmd(c)
			},
		},
		{
			Name: "get",
			Usage: "get allows for public information retrieval from a remote " +
				"drand node.\n",
			Subcommands: []cli.Command{
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
						"as specified in group.toml. Only one node is	contacted by " +
						"default. This command attempts to connect to the drand " +
						"beacon via TLS and falls back to plaintext communication " +
						"if the contacted node has not activated TLS in which case " +
						"it prints a warning.\n",
					Flags: toArray(tlsCertFlag, insecureFlag, roundFlag, nodeFlag),
					Action: func(c *cli.Context) error {
						return getPublicCmd(c)
					},
				},
				{
					Name: "cokey",
					Usage: "Get distributed public key generated during the " +
						" DKG step.",
					ArgsUsage: "<group.toml> provides the group informations of " +
						"the node that we are trying to contact.",
					Flags: toArray(tlsCertFlag, insecureFlag, nodeFlag),
					Action: func(c *cli.Context) error {
						return getCokeyCmd(c)
					},
				},
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
			Name: "show",
			Usage: "local information retrieval about the node's cryptographic " +
				"material. Show can print the information about the collective " +
				"public key (drand.cokey), the group details (group.toml), the " +
				"long-term private key (drand.private), the long-term public key " +
				"(drand.public), or the private key share (drand.share), " +
				"respectively.\n",
			Flags: toArray(controlFlag),
			Subcommands: []cli.Command{
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
					Flags: toArray(outFlag, controlFlag),
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
					Usage: "shows he long-term public key of a node.\n",
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
		if c.GlobalIsSet("verbose") {
			if c.Int("verbose") == 0 {
				slog.Level = slog.LevelPrint
			}
			if c.Int("verbose") == 1 {
				slog.Level = slog.LevelInfo
			}
			if c.Int("verbose") == 2 {
				slog.Level = slog.LevelDebug
			}
		}
		testWindows(c)
		return nil
	}
	app.Run(os.Args)
}

func resetBeaconDB(config *core.Config) bool {
	if _, err := os.Stat(config.DBFolder()); err == nil {
		// using fmt so does not get the new line at the end.
		// XXX allow slog for that behavior
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
	//slog.Print("asking for port")
	for {
		var port string
		slog.Print("No port given. Please, choose a port number (or ENTER for default port 8080): ")
		fmt.Scanf("%s\n", &port)
		if port == "" {
			return default_port
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
		slog.Fatal("TLS is not available on Windows, please disable TLS")
	}
}

func stopCmd(c *cli.Context) error {
	conf := contextToConfig(c)
	fs := key.NewFileStore(conf.ConfigFolder())
	var drand *core.Drand
	drand, err := core.LoadDrand(fs, conf)
	if err != nil {
		slog.Fatal(err)
	}
	drand.Stop()
	return nil
}

func keygenCmd(c *cli.Context) error {
	args := c.Args()
	if !args.Present() {
		slog.Fatal("Missing drand address in argument")
	}
	addr := args.First()
	var validID = regexp.MustCompile(`[:][0-9]+$`)
	if !validID.MatchString(addr) {
		slog.Print("port not ok")
		addr = addr + ":" + askPort()
	}
	var priv *key.Pair
	if c.Bool("tls-disable") {
		slog.Info("Generating private / public key pair without TLS.")
		priv = key.NewKeyPair(addr)
	} else {
		slog.Info("Generating private / public key pair with TLS indication")
		priv = key.NewTLSKeyPair(addr)
	}

	config := contextToConfig(c)
	fs := key.NewFileStore(config.ConfigFolder())

	if _, err := fs.LoadKeyPair(); err == nil {
		slog.Info("keypair already present. Remove them before generating new one")
		return nil
	}
	if err := fs.SaveKeyPair(priv); err != nil {
		slog.Fatal("could not save key: ", err)
	}
	fullpath := path.Join(config.ConfigFolder(), key.KeyFolderName)
	absPath, err := filepath.Abs(fullpath)
	if err != nil {
		slog.Fatal("err getting full path: ", err)
	}
	slog.Print("Generated keys at ", absPath)
	slog.Print("You can copy paste the following snippet to a common group.toml file:")
	var buff bytes.Buffer
	buff.WriteString("[[nodes]]\n")
	if err := toml.NewEncoder(&buff).Encode(priv.Public.TOML()); err != nil {
		panic(err)
	}
	buff.WriteString("\n")
	slog.Print(buff.String())
	slog.Print("Or just collect all public key files and use the group command!")
	return nil
}

func groupCmd(c *cli.Context) error {
	if !c.Args().Present() || (c.NArg() < 3 && !c.IsSet("group")) {
		slog.Fatal("drand: group command take at least 3 keys as arguments")
	}
	var threshold = key.DefaultThreshold(c.NArg())
	publics := make([]*key.Identity, c.NArg())
	for i, str := range c.Args() {
		pub := &key.Identity{}
		slog.Infof("drand: reading public identity from %s", str)
		if err := key.Load(str, pub); err != nil {
			slog.Fatal(err)
		}
		publics[i] = pub
	}

	var period = core.DefaultBeaconPeriod
	var err error
	if c.IsSet(periodFlag.Name) {
		period, err = time.ParseDuration(c.String(periodFlag.Name))
		if err != nil {
			slog.Fatalf("drand: invalid period time given %s", err)
		}
	}

	var group *key.Group
	if c.IsSet("group") {
		// merge with given group
		groupPath := c.String("group")
		oldG := &key.Group{}
		if err := key.Load(groupPath, oldG); err != nil {
			slog.Fatal(err)
		}
		group = oldG.MergeGroup(publics)
	} else {
		group = key.NewGroup(publics, threshold)
	}
	group.Period = period

	if c.IsSet("out") {
		groupPath := c.String("out")
		if err := key.Save(groupPath, group, false); err != nil {
			slog.Fatal(err)
		}
	} else {
		var buff bytes.Buffer
		if err := toml.NewEncoder(&buff).Encode(group.TOML()); err != nil {
			slog.Print("doesn't want to encode")
		}
		buff.WriteString("\n")
		slog.Printf("Copy the following snippet into a new group.toml file " +
			"and distribute it to all the participants:\n")
		slog.Printf(buff.String())
	}
	return nil
}

func toArray(flags ...cli.Flag) []cli.Flag {
	return flags
}

func getGroup(c *cli.Context) *key.Group {
	g := &key.Group{}
	if err := key.Load(c.Args().First(), g); err != nil {
		slog.Fatalf("drand: error loading group argument: %s", err)
	}
	slog.Infof("group file loaded with %d participants", g.Len())
	return g
}

func runDkg(c *cli.Context, d *core.Drand, ks key.Store) error {
	var err error
	if c.Bool("leader") {
		err = d.StartDKG()
	} else {
		err = d.WaitDKG()
	}
	if err != nil {
		slog.Fatal(err)
	}
	slog.Print("DKG setup finished!")
	public, err := ks.LoadDistPublic()
	if err != nil {
		slog.Fatal(err)
	}
	dir := fs.Pwd()
	p := path.Join(dir, dpublic)
	key.Save(p, public, false)
	slog.Print("distributed public key saved at ", p)
	return nil
}

// keyIdFromAddr looks at every node in the group file to retrieve to *key.Identity
func keyIdFromAddr(addr string, group *key.Group) *key.Identity {
	ids := group.Identities()
	for _, id := range ids {
		if id.Address() == addr {
			return id
		}
	}
	slog.Fatal("Could not retrive the node you are trying to contact in the group file.")
	return nil
}

func contextToConfig(c *cli.Context) *core.Config {
	var opts []core.ConfigOption
	listen := c.String("listen")
	if listen != "" {
		opts = append(opts, core.WithListenAddress(listen))
	}
	port := c.String(controlFlag.Name)
	if port != "" {
		opts = append(opts, core.WithControlPort(port))
	}
	config := c.GlobalString("folder")
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
	conf := core.NewConfig(opts...)
	return conf
}

func getNodes(c *cli.Context) []*key.Identity {
	group := getGroup(c)
	var ids []*key.Identity
	gids := group.Identities()
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
			slog.Fatalf("drand: addresses specified don't exist in group.toml")
		}
	} else {
		// select them all in order
		ids = gids
	}
	if len(ids) == 0 {
		slog.Fatalf("drand: no nodes specified with --nodes are in the group file")
	}
	return ids
}

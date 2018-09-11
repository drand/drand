// drand is a distributed randomness beacon. It provides periodically an
// unpredictable, bias-resistant, and verifiable random value.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dedis/drand/core"
	"github.com/dedis/drand/fs"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/dedis/drand/protobuf/drand"
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
const defaultListenPort = "8080"

func banner() {
	fmt.Printf("drand v%s by nikkolasg @ DEDIS\n", version)
	s := "WARNING: this software has NOT received a full audit and must be \n" +
		"used with caution and probably NOT in a production environment.\n"
	fmt.Printf(s)
}

func main() {
	app := cli.NewApp()
	app.Version = version

	// =====FLAGS=====

	folderFlag := cli.StringFlag{
		Name:  "folder, f",
		Value: core.DefaultConfigFolder(),
		Usage: "Folder to keep all drand cryptographic informations, in absolute form.",
	}
	leaderFlag := cli.BoolFlag{
		Name:  "leader",
		Usage: "Set this node as the initator of the distributed key generation process.",
	}
	verboseFlag := cli.IntFlag{
		Name:  "verbose, V",
		Value: 0,
		Usage: "Set verbosity to the given level.",
	}
	tlsCertFlag := cli.StringFlag{
		Name: "tls-cert, c",
		Usage: "Set the TLS certificate chain (in PEM format) for this drand node. " +
			"The certificates have to be specified as a list of whitespace-separated file paths. " +
			"This parameter is required by default and can only be omitted if the --tls-disable flag is used.",
	}
	tlsKeyFlag := cli.StringFlag{
		Name: "tls-key, k",
		Usage: "Set the TLS private key (in PEM format) for this drand node. " +
			"The keys have to be specified as a list of whitespace-separated file paths. " +
			"This parameter is required by default and can only be omitted if the --tls-disable flag is used.",
	}
	insecureFlag := cli.BoolFlag{
		Name:  "tls-disable, d",
		Usage: "Disable TLS for all communications (not recommended).",
	}
	portFlag := cli.StringFlag{
		Name:  "port",
		Usage: "Set the port you want to listen to for control port commands. If not specified, we will use the default port 8888.",
	}
	listenFlag := cli.StringFlag{
		Name:  "listen, l",
		Usage: "Set the listening (binding) address. Useful if you have some kind of proxy.",
	}
	nodeFlag := cli.StringFlag{
		Name:  "nodes, n",
		Usage: "Contact the nodes at the given list of whitespace-separated addresses which have to be present in group.toml.",
	}
	roundFlag := cli.IntFlag{
		Name:  "round, r",
		Usage: "Request the public randomness generated at round num. If the drand beacon does not have the requested value, it returns an error. If not specified, the current randomness is returned.",
	}

	// XXX deleted flags : debugFlag, outFlag, groupFlag, seedFlag, periodFlag, certsDirFlag, distKeyFlag, thresholdFlag.

	// =====Commands=====

	app.Commands = []cli.Command{
		cli.Command{
			Name: "start",
			Usage: "Start the drand daemon.\nIf the distributed key generation has not been executed before," +
				" the node waits to receive the signal from a leader to start the process of generating the collective public" +
				" key drand.cokey and its private share drand.share together with the other nodes in group.toml.\nOtherwise, " +
				"if there has been already a successful distributed key generation before, the node automatically switches to " +
				"the public randomness generation mode after a potential state-syncing phase with the other nodes in group.toml.",
			ArgsUsage: "<group.toml> the group file.",
			Flags:     toArray(leaderFlag, tlsCertFlag, tlsKeyFlag, insecureFlag, portFlag, listenFlag),
			Action: func(c *cli.Context) error {
				banner()
				return startCmd(c)
			},
		},
		cli.Command{
			Name:  "stop",
			Usage: "Stop the drand daemon.",
			Action: func(c *cli.Context) error {
				banner()
				return stopCmd(c)
			},
		},
		cli.Command{
			Name:      "generate-keypair",
			Usage:     "Generate the longterm keypair (drand.private, drand.public) for this node.",
			ArgsUsage: "<address> is the public address for other nodes to contact",
			Flags:     toArray(insecureFlag),
			Action: func(c *cli.Context) error {
				banner()
				return keygenCmd(c)
			},
		},
		cli.Command{
			Name: "group",
			Usage: "Merge the given list of whitespace-separated drand.public keys into the group.toml " +
				"file if one is provided, if not create a new group.toml file with the given identites.",
			ArgsUsage: "<key1 key2 key3...> must be the identities of the group to create/to insert into the group",
			Action: func(c *cli.Context) error {
				banner()
				return XXX(c)
			},
		},
		cli.Command{
			Name: "update",
			Usage: "Reshare the distributed key from the original set of nodes (old-group.toml) towards " +
				"a new set (new-group.toml).\nTo execute this resharing at least t-of-n nodes from the original group have " +
				"to be present. The new configuration can deviate arbitrarily from the old one including a different " +
				"number of nodes n' or recovery threshold t'.\nAfter the resharing has been finished successfully, all " +
				"nodes in the new group switch to the public randomness generation mode while all nodes in the original " +
				"group delete their outdated private key shares.",
			ArgsUsage: "<ld-group.toml> <new-group.toml>",
			Flags:     toArray(leaderFlag),
			Action: func(c *cli.Context) error {
				banner()
				return XXX(c)
			},
		},
		{
			Name:  "get",
			Usage: "Interactions with a remote drand node.",
			Subcommands: []cli.Command{
				{
					Name: "private",
					Usage: "Get private randomness from the drand beacon as specified in group.toml. " +
						"Only one node is contacted by default. Requests are ECIES-encrypted towards the public " +
						"key of the contacted node. This command attempts to connect to	the drand beacon via TLS " +
						"and falls back to plaintext communication if the	contacted node has not activated TLS in " +
						"which case it prints a warning.",
					ArgsUsage: "<group.toml> provides the group informations of the node that we are trying to contact.",
					Flags:     toArray(tlsCertFlag, nodeFlag),
					Action: func(c *cli.Context) error {
						return getPrivateCmd(c)
					},
				},
				{
					Name: "public",
					Usage: "Get the latest public randomness from the drand beacon and verify it against the " +
						"collective public key as specified in group.toml. Only one node is	contacted by default. This " +
						"command attempts to connect to the drand beacon via TLS and falls back to plaintext communication " +
						"if the contacted node has not activated TLS in which case it prints a warning.",
					Flags: toArray(tlsCertFlag, insecureFlag, roundFlag, nodeFlag),
					Action: func(c *cli.Context) error {
						return getPublicCmd(c)
					},
				},
				{
					Name:      "cokey",
					Usage:     "Get distributed key generated dring the DKG step.",
					ArgsUsage: "<group.toml> provides the group informations of the node that we are trying to contact.",
					Flags:     toArray(tlsCertFlag, nodeFlag),
					Action: func(c *cli.Context) error {
						return getCokeyCmd(c)
					},
				},
			},
		},
		{
			Name: "show",
			Usage: "Print the information about the collective public key (drand.cokey), the " +
				"group details (group.toml), the long-term private key (drand.private), the long-term " +
				"public key (drand.public), or the private key share (drand.share), respectively.",
			Flags: toArray(portFlag),
			Subcommands: []cli.Command{
				{
					Name: "share",
					Action: func(c *cli.Context) error {
						return showShareCmd(c)
					},
				},
				{
					Name:  "group",
					Usage: "Returns the group.toml.",
					Action: func(c *cli.Context) error {
						return XXX(c)
					},
				},
				{
					Name:  "cokey",
					Usage: "Returns the collective key generated during DKG.",
					Action: func(c *cli.Context) error {
						return showCokeyCmd(c)
					},
				},
				{
					Name:  "private",
					Usage: "Returns the long-term private key of a node.",
					Action: func(c *cli.Context) error {
						return showPrivateCmd(c)
					},
				},
				{
					Name:  "public",
					Usage: "Returns the long-term public key of a node.",
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
			if c.Int("verbose") == 1 {
				slog.Level = slog.LevelInfo
			}
			if c.Int("verbose") == 2 {
				slog.Level = slog.LevelPrint
			}
			if c.Int("verbose") == 3 {
				slog.Level = slog.LevelDebug
			}
		}
		return nil
	}
	app.Run(os.Args)
}

// XXX deleted commands : dkg, beacon

// =====Functions=====

func XXX(c *cli.Context) error {
	slog.Print("not implemented yet")
	return nil
}

func testWindows(c *cli.Context) {
	//x509 not available on windows: must run without TLS
	if runtime.GOOS == "windows" && !c.Bool("tls-disable") {
		slog.Fatal("TLS is not available on Windows, please disable TLS")
	}
}

func startCmd(c *cli.Context) error {
	testWindows(c)
	conf := contextToConfig(c)
	fs := key.NewFileStore(conf.ConfigFolder())
	var drand *core.Drand
	var err error
	if c.Args().Present() {
		group := getGroup(c)
		if exit := resetBeaconDB(conf); exit {
			os.Exit(0)
		}
		drand, err = core.NewDrand(fs, group, conf)
		if err != nil {
			slog.Fatal(err)
		}
		slog.Print("Starting the dkg first.")
		runDkg(c, drand, fs)
	} else {
		_, errG := fs.LoadGroup()
		_, errS := fs.LoadShare()
		_, errD := fs.LoadDistPublic()
		if errG != nil || errS != nil || errD != nil {
			slog.Fatalf("The DKG has not been run before, please provide a group file to do the setup.")
		}
		slog.Print("No group file given, drand will try to run as a beacon.")
		drand, err = core.LoadDrand(fs, conf)
		if err != nil {
			slog.Fatal(err)
		}
	}
	slog.Print("Running the randomness beacon...")
	drand.BeaconLoop()
	return nil
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
	testWindows(c)
	args := c.Args()
	if !args.Present() {
		slog.Fatal("Missing drand address in argument (IPv4, dns)")
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

// getPrivateCmd constructs a *key.Identity struct and uses grpc com
func getPrivateCmd(c *cli.Context) error {
	if !c.Args().Present() {
		slog.Fatal("Get private takes a group file as argument.")
	}
	if !c.IsSet("nodes") {
		slog.Fatal("Get private needs to know the address of the server to contact.")
	}
	defaultManager := net.NewCertManager()
	if c.IsSet("tls-cert") {
		defaultManager.Add(c.String("tls-cert"))
	}
	addr := c.String("nodes")
	group := getGroup(c)
	public := keyIdFromAddr(addr, group)
	client := core.NewGrpcClientFromCert(defaultManager)
	resp, err := client.Private(public)
	if err != nil {
		slog.Fatal(err)
	}
	type private struct {
		Randomness []byte `json:"randomness"`
	}
	buff, err := json.MarshalIndent(&private{resp}, "", "    ")
	if err != nil {
		slog.Fatal("could not JSON marshal:", err)
	}
	slog.Print(string(buff))
	return nil
}

func getPublicCmd(c *cli.Context) error {
	if !c.Args().Present() {
		slog.Fatal("Get public command takes a group file as argument.")
	}
	if !c.IsSet("nodes") {
		slog.Fatal("Get public command needs to know the address of the server to contact.")
	}
	defaultManager := net.NewCertManager()
	if c.IsSet("tls-cert") {
		defaultManager.Add(c.String("tls-cert"))
	}
	addr := c.String("nodes")
	group := getGroup(c)
	public := group.GetCoKey()
	client := core.NewGrpcClientFromCert(defaultManager)
	var resp *drand.PublicRandResponse
	var err error
	if c.IsSet("round") {
		resp, err = client.Public(addr, public, !c.Bool("tls-disable"), c.Int("round"))
		if err != nil {
			slog.Fatal("could not get verified randomness:", err)
		}
	} else {
		resp, err = client.LastPublic(addr, public, !c.Bool("tls-disable"))
		if err != nil {
			slog.Fatal("could not get verified randomness:", err)
		}
	}
	buff, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		slog.Fatal("could not JSON marshal:", err)
	}
	slog.Print(string(buff))
	return nil
}

func getCokeyCmd(c *cli.Context) error {
	if !c.IsSet("nodes") {
		slog.Fatal("Get private needs to know the address of the server to contact.")
	}
	defaultManager := net.NewCertManager()
	if c.IsSet("tls-cert") {
		defaultManager.Add(c.String("tls-cert"))
	}
	addr := c.String("nodes")
	client := core.NewGrpcClientFromCert(defaultManager)
	key, err := client.DistKey(addr, !c.Bool("tls-disable"))
	if err != nil {
		slog.Fatal("could not fetch the distributed key from that server:", err)
	}
	b, _ := key.MarshalBinary()
	type dkey struct {
		CollectiveKey []byte `json:"collective key"`
	}
	buff, err := json.MarshalIndent(&dkey{b}, "", "    ")
	if err != nil {
		slog.Fatal("could not JSON marshal:", err)
	}
	slog.Print(string(buff))
	return nil
}

func showShareCmd(c *cli.Context) error {
	port := c.String("port")
	if port == "" {
		port = core.DefaultControlPort
	}
	client := net.NewControlClient(port)
	resp, err := client.Share()
	if err != nil {
		slog.Fatalf("drand: could not request drand.share: %s", err)
	}
	buff, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		slog.Fatal("could not JSON marshal:", err)
	}
	slog.Print(string(buff))
	return nil
}

func showPublicCmd(c *cli.Context) error {
	port := c.String("port")
	if port == "" {
		port = core.DefaultControlPort
	}
	client := net.NewControlClient(port)
	resp, err := client.PublicKey()
	if err != nil {
		slog.Fatalf("drand: could not request drand.public: %s", err)
	}
	buff, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		slog.Fatal("could not JSON marshal:", err)
	}
	slog.Print(string(buff))
	return nil
}

func showPrivateCmd(c *cli.Context) error {
	port := c.String("port")
	if port == "" {
		port = core.DefaultControlPort
	}
	client := net.NewControlClient(port)
	resp, err := client.PrivateKey()
	if err != nil {
		slog.Fatalf("drand: could not request drand.private: %s", err)
	}
	buff, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		slog.Fatal("could not JSON marshal:", err)
	}
	slog.Print(string(buff))
	return nil
}

func showCokeyCmd(c *cli.Context) error {
	port := c.String("port")
	if port == "" {
		port = core.DefaultControlPort
	}
	client := net.NewControlClient(port)
	resp, err := client.CollectiveKey()
	if err != nil {
		slog.Fatalf("drand: could not request drand.cokey: %s", err)
	}
	buff, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		slog.Fatal("could not JSON marshal:", err)
	}
	slog.Print(string(buff))
	return nil
}

func toArray(flags ...cli.Flag) []cli.Flag {
	return flags
}

func askPort() string {
	for {
		var port string
		slog.Print("No port given. Please, choose a port number (or ENTER for default port 8080): ")
		fmt.Scanf("%s\n", &port)
		if port == "" {
			return defaultListenPort
		}
		_, err := strconv.Atoi(port)
		if len(port) > 2 && len(port) < 5 && err == nil {
			return port
		}
		return askPort()
	}
}

func getGroup(c *cli.Context) *key.Group {
	g := &key.Group{}
	if err := key.Load(c.Args().First(), g); err != nil {
		slog.Fatal(err)
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

func resetBeaconDB(config *core.Config) bool {
	if _, err := os.Stat(config.DBFolder()); err == nil {
		// using fmt so does not get the new line at the end. XXX allow slog for that behavior
		fmt.Print("INCONSISTENT STATE: the group-init flag is set, but a beacon database exists already.\ndrand support only one identity at the time and thus needs to delete the existing beacon database.\nAccept to delete database ? [y/n]: ")
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			slog.Fatal("error reading: ", err)
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" {
			slog.Print("Not deleting the database. Exiting drand.")
			return true
		}
		if err := os.RemoveAll(config.DBFolder()); err != nil {
			slog.Fatal(err)
		}
		slog.Print("Removed existing beacon database.")
	}
	return false
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
	port := c.String("port")
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
	conf := core.NewConfig(opts...)
	return conf
}

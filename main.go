// drand is a distributed randomness beacon. It provides periodically an
// unpredictable, bias-resistant, and verifiable random value.
package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
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
	fmt.Printf("drand v%s by nikkolasg @ DEDIS\n", version)
	s := "WARNING: this software has NOT received a full audit and must be \n" +
		"used with caution and probably NOT in a production environment.\n"
	fmt.Printf(s)
}

func main() {
	app := cli.NewApp()
	app.Version = version
	configFlag := cli.StringFlag{
		Name:  "config, c",
		Value: core.DefaultConfigFolder(),
		Usage: "Folder to keep all drand cryptographic informations, in absolute form.",
	}
	seedFlag := cli.StringFlag{
		Name:  "seed",
		Value: string(core.DefaultSeed),
		Usage: "set the seed message of the first beacon produced",
	}
	periodFlag := cli.DurationFlag{
		Name:  "period",
		Value: core.DefaultBeaconPeriod,
		Usage: "runs the beacon every `PERIOD`",
	}
	leaderFlag := cli.BoolFlag{
		Name:  "leader",
		Usage: "Leader is the first node to start the DKG protocol",
	}
	verboseFlag := cli.BoolFlag{
		Name:  "debug, d",
		Usage: "Use -d to log debug output",
	}
	listenFlag := cli.StringFlag{
		Name:  "listen,l",
		Usage: "listening (binding) address. Useful if you have some kind of proxy",
	}
	distKeyFlag := cli.StringFlag{
		Name:  "public,p",
		Usage: "the path of the public key file",
	}
	thresholdFlag := cli.IntFlag{
		Name:  "threshold, t",
		Usage: "threshold to apply for the group. Default is n/2 + 1.",
	}
	outFlag := cli.StringFlag{
		Name:  "out, o",
		Usage: "where to save either the group file or the distributed public key",
	}

	tlsCertFlag := cli.StringFlag{
		Name:  "tls-cert",
		Usage: "TLS certificate path to use",
	}
	tlsKeyFlag := cli.StringFlag{
		Name:  "tls-key",
		Usage: "TLS private key to use by the server",
	}
	certsDirFlag := cli.StringFlag{
		Name:  "certs-dir",
		Usage: "directory containing trusted certificates. Useful for testing and self signed certificates",
	}
	insecureFlag := cli.BoolFlag{
		Name:  "insecure",
		Usage: "indicates to use a non TLS server or connection",
	}

	groupFlag := cli.StringFlag{
		Name:  "group-init",
		Usage: "the group file to use during the DKG. If specified, drand erases any existing beacon database, as it supports only being part of one group at a time.",
	}

	app.Commands = []cli.Command{
		cli.Command{
			Name:      "keygen",
			Usage:     "keygen <ADDRESS>. Generates longterm private key pair",
			ArgsUsage: "ADDRESS is the public address for other nodes to contact",
			Flags:     toArray(insecureFlag),
			Action: func(c *cli.Context) error {
				banner()
				return keygenCmd(c)
			},
		},
		cli.Command{
			Name:      "group",
			Usage:     "Create the group toml from individual public keys",
			ArgsUsage: "<id1 id2 id3...> must be the identities of the group to create",
			Flags:     toArray(thresholdFlag, outFlag),
			Action: func(c *cli.Context) error {
				banner()
				return groupCmd(c)
			},
		},
		cli.Command{
			Name:  "dkg",
			Usage: "Run the DKG protocol",
			Flags: toArray(leaderFlag, listenFlag, tlsCertFlag, tlsKeyFlag, certsDirFlag, groupFlag),
			Action: func(c *cli.Context) error {
				banner()
				return dkgCmd(c)
			},
		},
		cli.Command{
			Name:  "beacon",
			Usage: "Run the beacon protocol",
			Flags: toArray(periodFlag, seedFlag, listenFlag, tlsCertFlag, tlsKeyFlag, certsDirFlag),
			Action: func(c *cli.Context) error {
				banner()
				return beaconCmd(c)
			},
		},
		cli.Command{
			Name:  "run",
			Usage: "Run the daemon, first do the dkg if needed then run the beacon",
			Flags: toArray(leaderFlag, periodFlag, seedFlag, listenFlag, tlsCertFlag, tlsKeyFlag, certsDirFlag, insecureFlag, groupFlag),
			Action: func(c *cli.Context) error {
				banner()
				return runCmd(c)
			},
		},
		{
			Name:    "fetch",
			Aliases: []string{"f"},
			Usage:   "fetch some randomness",
			Subcommands: []cli.Command{
				{
					Name:      "public",
					Usage:     "Fetch a public verifiable and unbiasable randomness value",
					ArgsUsage: "<server address> address of the server to contact",
					Flags:     toArray(distKeyFlag, tlsCertFlag, insecureFlag, certsDirFlag),
					Action: func(c *cli.Context) error {
						return fetchPublicCmd(c)
					},
				},
				{
					Name:      "private",
					Usage:     "Fetch a private randomness from a server. Request and response are encrypted",
					ArgsUsage: "<identity file> identity file of the remote server",
					Flags:     toArray(tlsCertFlag, certsDirFlag),
					Action: func(c *cli.Context) error {
						return fetchPrivateCmd(c)
					},
				},
				{
					Name:      "dist_key",
					Usage:     "Fetch the distributed public key from a server.",
					ArgsUsage: "<server address> address of the server to contact",
					Flags:     toArray(tlsCertFlag, certsDirFlag, insecureFlag),
					Action: func(c *cli.Context) error {
						return fetchDistKey(c)
					},
				},
			},
		},
		{
			Name:  "control",
			Usage: "doing secret stuff locally",
			Subcommands: []cli.Command{
				{
					Name:  "share",
					Usage: "Returns the private share of a node.",
					Action: func(c *cli.Context) error {
						return controlShare(c)
					},
				},
			},
		},
	}
	app.Flags = toArray(verboseFlag, configFlag)
	app.Before = func(c *cli.Context) error {
		if c.GlobalIsSet("debug") {
			slog.Level = slog.LevelDebug
		}
		return nil
	}
	app.Run(os.Args)
}

func keygenCmd(c *cli.Context) error {
	//x509 not available on windows: must run in insecure mode
	if runtime.GOOS == "windows" && !c.Bool("insecure") {
		slog.Fatal("TLS is not available on Windows, please run in insecure mode")
	}
	args := c.Args()
	if !args.Present() {
		slog.Fatal("Missing drand address in argument (IPv4, dns)")
	}
	addr := args.First()
	var validID = regexp.MustCompile(`[:][0-9]+$`)
	slog.Print("Testing port")
	if !validID.MatchString(addr) {
		slog.Print("port not ok")
		addr = addr + ":" + askPort()
	}
	var priv *key.Pair
	if c.Bool("insecure") {
		slog.Info("Generating private / public key pair in INSECURE mode (no TLS).")
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

// groupCmd reads the identity, check the threshold and outputs the group.toml
// file
func groupCmd(c *cli.Context) error {
	args := c.Args()
	if !args.Present() {
		slog.Fatal("missing identity file to create the group.toml")
	}
	if c.NArg() < 3 {
		slog.Fatal("not enough identities (", c.NArg(), ") to create a group toml. At least 3!")
	}
	var threshold = key.DefaultThreshold(c.NArg())
	if c.IsSet("threshold") {
		if c.Int("threshold") < threshold {
			slog.Print("WARNING: You are using a threshold which is TOO LOW.")
			slog.Print("		 It should be at least ", threshold)
		}
		threshold = c.Int("threshold")
	}

	publics := make([]*key.Identity, c.NArg())
	for i, str := range args {
		pub := &key.Identity{}
		slog.Print("Reading public identity from ", str)
		if err := key.Load(str, pub); err != nil {
			slog.Fatal(err)
		}
		publics[i] = pub
	}
	group := key.NewGroup(publics, threshold)
	groupPath := path.Join(fs.Pwd(), gname)
	if c.String("out") != "" {
		groupPath = c.String("out")
	}
	if err := key.Save(groupPath, group, false); err != nil {
		slog.Fatal(err)
	}
	slog.Printf("Group file written in %s. Distribute it to all the participants to start the DKG", groupPath)
	return nil
}

func dkgCmd(c *cli.Context) error {
	if !c.IsSet("group-init") {
		slog.Fatal("dkg requires a group.toml file")
	}
	group := getGroup(c)
	conf := contextToConfig(c)
	if exit := resetBeaconDB(conf); exit {
		os.Exit(0)
	}
	fs := key.NewFileStore(conf.ConfigFolder())
	drand, err := core.NewDrand(fs, group, conf)
	if err != nil {
		slog.Fatal(err)
	}
	return runDkg(c, drand, fs)
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

func beaconCmd(c *cli.Context) error {
	conf := contextToConfig(c)
	fs := key.NewFileStore(conf.ConfigFolder())
	drand, err := core.LoadDrand(fs, conf)
	if err != nil {
		slog.Fatal(err)
	}
	drand.BeaconLoop()
	return nil
}

func runCmd(c *cli.Context) error {
	//x509 not available on windows: must run in insecure mode
	if runtime.GOOS == "windows" && !c.Bool("insecure") {
		slog.Fatal("TLS is not available on Windows, please run in insecure mode")
	}
	conf := contextToConfig(c)
	fs := key.NewFileStore(conf.ConfigFolder())
	var drand *core.Drand
	var err error
	if c.IsSet("group-init") {
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

func fetchPrivateCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		slog.Fatal("fetch private takes the identity file of a server to contact")
	}
	public := &key.Identity{}
	if err := key.Load(c.Args().First(), public); err != nil {
		slog.Fatal(err)
	}
	slog.Info("contacting public drand node: ", public.Address())
	defaultManager := net.NewCertManager()
	if c.IsSet("tls-cert") {
		defaultManager.Add(c.String("tls-cert"))
	}
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

func fetchPublicCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		slog.Fatal("fetch command takes the address of a server to contact")
	}

	public := &key.DistPublic{}
	if err := key.Load(c.String("public"), public); err != nil {
		slog.Fatal(err)
	}
	defaultManager := net.NewCertManager()
	if c.IsSet("tls-cert") {
		defaultManager.Add(c.String("tls-cert"))
	}
	client := core.NewGrpcClientFromCert(defaultManager)
	resp, err := client.LastPublic(c.Args().First(), public, !c.Bool("insecure"))
	if err != nil {
		slog.Fatal("could not get verified randomness:", err)
	}
	buff, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		slog.Fatal("could not JSON marshal:", err)
	}
	slog.Print(string(buff))
	return nil
}

func fetchDistKey(c *cli.Context) error {
	if c.NArg() < 1 {
		slog.Fatal("fetch dist_key command takes the address of a server to contact")
	}
	defaultManager := net.NewCertManager()
	if c.IsSet("tls-cert") {
		defaultManager.Add(c.String("tls-cert"))
	}
	client := core.NewGrpcClientFromCert(defaultManager)
	key, err := client.DistKey(c.Args().First(), !c.Bool("insecure"))
	if err != nil {
		slog.Fatal("could not fetch the distributed key from that server:", err)
	}
	b, _ := key.MarshalBinary()
	dst := make([]byte, hex.EncodedLen(len(b)))
	hex.Encode(dst, b)
	slog.Print("{\n    \"distributed key\": \"" + string(dst) + "\"\n}")
	return nil
}

func controlShare(c *cli.Context) error {
	// TODO:
	return nil
}

func toArray(flags ...cli.Flag) []cli.Flag {
	return flags
}

func contextToConfig(c *cli.Context) *core.Config {
	var opts []core.ConfigOption
	listen := c.String("listen")
	if listen != "" {
		opts = append(opts, core.WithListenAddress(listen))
	}

	config := c.GlobalString("config")
	opts = append(opts, core.WithConfigFolder(config))
	period := c.Duration("period")
	opts = append(opts, core.WithBeaconPeriod(period))

	if c.Bool("insecure") {
		opts = append(opts, core.WithInsecure())
		if c.IsSet("tls-cert") || c.IsSet("tls-key") {
			panic("option 'insecure' used with 'tls-cert' or 'tls-key': combination is not valid")
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
		fmt.Println("certs-dirs files: ", strings.Join(paths, ","))
		opts = append(opts, core.WithTrustedCerts(paths...))
	}

	if c.IsSet("certs-dir") {
		core.WithTrustedCerts(c.String("certs-dir"))
	}

	conf := core.NewConfig(opts...)
	return conf
}

func getGroup(c *cli.Context) *key.Group {
	g := &key.Group{}
	if err := key.Load(c.String("group-init"), g); err != nil {
		slog.Fatal(err)
	}
	slog.Infof("group file loaded with %d participants", g.Len())
	return g
}

func resetBeaconDB(config *core.Config) bool {
	if _, err := os.Stat(config.DBFolder()); err == nil {
		// using fmt so does not get the new line at the end.
		// XXX allow slog for that behavior
		fmt.Print("INCONSISTENT STATE: the group-init flag is set, but a beacon database exists already.\ndrand support only one identity at the time and thus needs to delete the existing beacon database.\nAccept to delete database ? [Y/n]: ")
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

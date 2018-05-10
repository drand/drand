// drand is a distributed randomness beacon. It provides periodically an
// unpredictable, bias-resistant, and verifiable random value.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/BurntSushi/toml"
	"github.com/dedis/drand/core"
	"github.com/dedis/drand/key"
	"github.com/nikkolasg/slog"
	"github.com/urfave/cli"
)

const version = "0.2"
const gname = "group.toml"

func banner() {
	fmt.Printf("drand v%s-test by nikkolasg @ DEDIS, EPFL\n", version)
	s := "WARNING: this software has NOT received a full audit and must be \n" +
		"used with caution and probably NOT in a production environment.\n"
	fmt.Printf(s)
}

func main() {
	app := cli.NewApp()
	app.Version = version
	configFlag := cli.StringFlag{
		Name:  "config, c",
		Value: core.DefaultConfigFolder,
		Usage: "Folder to keep all drand cryptographic informations",
	}
	dbFlag := cli.StringFlag{
		Name:  "db",
		Value: path.Join(configFlag.Value, core.DefaultDbFolder),
		Usage: "Folder in which to keep the database",
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
		Usage: "the path of the distributed public key file",
	}
	thresholdFlag := cli.IntFlag{
		Name:  "threshold, t",
		Usage: "threshold to apply for the group. Default is n/2 + 1.",
	}

	app.Commands = []cli.Command{
		cli.Command{
			Name:      "keygen",
			Usage:     "keygen <ADDRESS>. Generates longterm private key pair",
			ArgsUsage: "ADDRESS is the public address for other nodes to contact",
			Action: func(c *cli.Context) error {
				banner()
				return keygenCmd(c)
			},
		},
		cli.Command{
			Name:      "group",
			Usage:     "Create the group toml from individual public keys",
			ArgsUsage: "<id1 id2 id3...> must be the identities of the group to create",
			Flags:     toArray(thresholdFlag),
			Action: func(c *cli.Context) error {
				banner()
				return groupCmd(c)
			},
		},
		cli.Command{
			Name:      "dkg",
			Usage:     "Run the DKG protocol",
			ArgsUsage: "GROUP.TOML the group file listing all participant's identities",
			Flags:     toArray(leaderFlag, listenFlag),
			Action: func(c *cli.Context) error {
				return dkgCmd(c)
			},
		},
		cli.Command{
			Name:  "beacon",
			Usage: "Run the beacon protocol",
			Flags: toArray(periodFlag, seedFlag, listenFlag),
			Action: func(c *cli.Context) error {
				return beaconCmd(c)
			},
		},
		cli.Command{
			Name:  "run",
			Usage: "Run the daemon, first do the dkg if needed then run the beacon",
			Flags: toArray(leaderFlag, periodFlag, seedFlag, listenFlag),
			Action: func(c *cli.Context) error {
				return runCmd(c)
			},
		},
		cli.Command{
			Name:      "fetch",
			Usage:     "Fetch a random beacon and verifies it",
			ArgsUsage: "<server address> address of the server to contact",
			Flags:     toArray(distKeyFlag),
			Action: func(c *cli.Context) error {
				return fetchCmd(c)
			},
		},
	}
	app.Flags = toArray(verboseFlag, configFlag, dbFlag)
	app.Before = func(c *cli.Context) error {
		banner()
		if c.GlobalIsSet("debug") {
			slog.Level = slog.LevelDebug
		}
		return nil
	}
	app.Run(os.Args)
}

func keygenCmd(c *cli.Context) error {
	args := c.Args()
	if !args.Present() {
		slog.Fatal("Missing peer address in argument")
	}
	priv := key.NewKeyPair(args.First())
	config := contextToConfig(c)
	fs := key.NewFileStore(config.ConfigFolder())
	if err := fs.SavePrivate(priv); err != nil {
		slog.Fatal("could not save key: ", err)
	}
	fullpath := path.Join(config.ConfigFolder(), key.KeyFolderName)
	slog.Print("Generated keys at ", fullpath)
	slog.Print("You can copy paste the following snippet to a common group.toml file:")
	var buff bytes.Buffer
	buff.WriteString("[[nodes]]\n")
	if err := toml.NewEncoder(&buff).Encode(priv.Public.TOML()); err != nil {
		panic(err)
	}
	buff.WriteString("\n")
	slog.Print(buff.String())
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
		if err := key.Load(str, pub); err != nil {
			slog.Fatal(err)
		}
		publics[i] = pub
	}
	group := key.NewGroup(publics, threshold)
	fd, err := os.Create(gname)
	defer fd.Close()
	if err != nil {
		slog.Fatalf("can't write to %s: %s", gname, err)
	}
	if err := toml.NewEncoder(fd).Encode(group.TOML()); err != nil {
		slog.Fatalf("can't write to %s: %s", gname, err)
	}
	slog.Printf("group file written in %s. Distribute it to all the participants to start the DKG")
	return nil
}

func dkgCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		slog.Fatal("dkg requires a group.toml file")
	}
	group := getGroup(c)
	conf := contextToConfig(c)
	fs := key.NewFileStore(conf.ConfigFolder())
	drand, err := core.NewDrand(fs, group, conf)
	if err != nil {
		slog.Fatal(err)
	}
	return runDkg(c, drand)
}

func runDkg(c *cli.Context, d *core.Drand) error {
	if c.Bool("leader") {
		return d.StartDKG()
	}
	return d.WaitDKG()
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
	conf := contextToConfig(c)
	fs := key.NewFileStore(conf.ConfigFolder())
	var drand *core.Drand
	var err error
	if c.NArg() > 0 {
		// we assume it is the group file
		group := getGroup(c)
		drand, err = core.NewDrand(fs, group, conf)
		if err != nil {
			slog.Fatal(err)
		}
		slog.Print("Starting the dkg first.")
		runDkg(c, drand)
	} else {
		drand, err = core.LoadDrand(fs, conf)
		if err != nil {
			slog.Fatal(err)
		}
		slog.Print("Running as randomness beacon.")
	}
	drand.BeaconLoop()
	return nil
}

func fetchCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		slog.Fatal("fetch command takes the address of a server to contact")
	}

	public := &key.DistPublic{}
	if err := key.Load(c.String("public"), public); err != nil {
		slog.Fatal(err)
	}
	conf := contextToConfig(c)
	client := core.NewClient(conf, public, c.Args().First())
	resp, err := client.Last()
	if err != nil {
		slog.Fatal("could not get verified randomness:", err)
	}
	buff, err := json.Marshal(resp)
	if err != nil {
		slog.Fatal("could not JSON marshal:", err)
	}
	slog.Print(buff)
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
	db := c.String("db")
	opts = append(opts, core.WithDbFolder(db))
	period := c.Duration("period")
	opts = append(opts, core.WithBeaconPeriod(period))
	return core.NewConfig(opts...)
}

func getGroup(c *cli.Context) *key.Group {
	g := &key.Group{}
	if err := key.Load(c.Args().First(), g); err != nil {
		slog.Fatal(err)
	}
	return g
}

package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/nikkolasg/slog"
	"github.com/urfave/cli"
)

const version = "0.1"
const defaultSeed = "Expose yourself to your deepest fear; after that," +
	" fear has no power, and the fear of freedom shrinks and vanishes. " +
	" You are free. Morrisson"
const defaultPeriod = 30 * time.Minute

func banner() {
	fmt.Printf("drand v%s-test by nikkolasg @ DEDIS, EPFL\n", version)
	s := "WARNING: this software has NOT received a full audit and must be \n" +
		"used with caution and probably NOT in a production environment.\n"
	fmt.Printf(s)
}

func main() {
	banner()
	app := cli.NewApp()
	app.Version = version
	// global flags re-used in many commands
	privFlag := cli.StringFlag{
		Name:  flagNameStruct(keyFolderFlagName),
		Value: appData(),
		Usage: "Key folder path.Private key must be in the folder under the name drand_id.private, public identity under the name drand_id.public",
	}
	groupFlag := cli.StringFlag{
		Name:  flagNameStruct(groupFileFlagName),
		Value: defaultGroupFile(),
		Usage: "group file listing identities of participants",
	}
	shareFlag := cli.StringFlag{
		Name:  flagNameStruct(shareFile(defaultGroupFile())),
		Value: shareFile(defaultGroupFile()),
		Usage: "private share file path of the group",
	}
	sigFlag := cli.StringFlag{
		Name:  flagNameStruct(sigFolderFlagName),
		Value: defaultSigFolder(),
		Usage: "folder where beacon stores the signatures",
	}
	leaderFlag := cli.BoolFlag{
		Name:  "leader, l",
		Usage: "use this flag if this node must start the protocol",
	}
	seedFlag := cli.StringFlag{
		Name:  "seed",
		Value: defaultSeed,
		Usage: "set the seed message of the first beacon produced (leader only)",
	}
	periodFlag := cli.DurationFlag{
		Name:  "period",
		Value: defaultPeriod,
		Usage: "runs the beacon every `PERIOD` seconds",
	}

	app.Commands = []cli.Command{
		cli.Command{
			Name:      "keygen",
			Aliases:   []string{"k"},
			Flags:     toArray(privFlag),
			Usage:     "keygen <address to listen>. Generates longterm private key pair",
			ArgsUsage: "ADDRESS must be a valid TCP ip address to listen on",
			Action: func(c *cli.Context) error {
				return keygenCmd(c)
			},
		},
		cli.Command{
			Name:      "group",
			Aliases:   []string{"g"},
			Usage:     "Create the group toml from individual public keys",
			ArgsUsage: "<id1 id2 id3...> must be the identities of the group to create",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "threshold, t",
					Usage: "threshold to apply for the group. Default is n/2 + 1.",
				},
				groupFlag,
			},
			Action: func(c *cli.Context) error {
				return groupCmd(c)
			},
		},
		cli.Command{
			Name:    "dkg",
			Aliases: []string{"d"},
			Usage:   "Run the DKG protocol",
			Flags:   toArray(privFlag, groupFlag, shareFlag, leaderFlag),
			Action: func(c *cli.Context) error {
				return dkgCmd(c)
			},
		},
		cli.Command{
			Name:    "beacon",
			Aliases: []string{"b"},
			Usage:   "Run the beacon protocol",
			Flags: toArray(privFlag, groupFlag, shareFlag, sigFlag,
				leaderFlag, periodFlag, seedFlag),
			Action: func(c *cli.Context) error {
				return beaconCmd(c)
			},
		},
		cli.Command{
			Name:    "run",
			Aliases: []string{"r"},
			Usage:   "Run the daemon, first do the dkg then run the beacon",
			Flags: toArray(privFlag, groupFlag, shareFlag, sigFlag,
				leaderFlag, periodFlag, seedFlag),
			Action: func(c *cli.Context) error {
				return runCmd(c)
			},
		},
	}
	app.Run(os.Args)
}

func keygenCmd(c *cli.Context) error {
	args := c.Args()
	if !args.Present() {
		slog.Fatal("Missing ip address argument")
	}
	if !isValidIP(args.First()) {
		slog.Print("IP address must be of the form <address>:<port> with port > 1000")
		slog.Fatal("Address given is not a valid ip address")
	}

	priv := NewKeyPair(args.First())
	fs := NewFileStore(c)
	if err := fs.SaveKey(priv); err != nil {
		slog.Fatal("could not save key: ", err)
	}
	slog.Print("Generated private key at ", fs.KeyFile)
	slog.Print("Generated public key at ", fs.PublicFile)
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
		slog.Fatal("not enough identities to create a group toml. At least 3!")
	}
	var threshold = defaultThreshold(c.NArg())
	if c.IsSet("threshold") {
		if c.Int("threshold") < threshold {
			slog.Print("WARNING: You are using a threshold which is TOO LOW.")
			slog.Print("		 It should be at least ", threshold)
		}
		threshold = c.Int("threshold")
	}

	publics := make([]*Public, c.NArg())
	for i, str := range args {
		ptoml := &PublicTOML{}
		_, err := toml.DecodeFile(str, ptoml)
		if err != nil {
			slog.Fatal("arg: ", str, " error: ", err)
		}
		pub := new(Public)
		if err := pub.FromTOML(ptoml); err != nil {
			slog.Fatal("arg: ", str, " error: ", err)
		}
		publics[i] = pub
	}
	indexed := toIndexedList(publics)
	group := Group{
		Threshold: threshold,
		Nodes:     indexed,
	}
	groupName := defaultGroupFile()
	if c.IsSet(groupFileFlagName) {
		groupName = c.String(groupFileFlagName)
	}
	fd, err := os.Create(groupName)
	if err != nil {
		slog.Fatal("error creating group file: ", err)
	}
	if err := toml.NewEncoder(fd).Encode(group.TOML()); err != nil {
		slog.Fatal("error writing to the group file:", err)
	}
	slog.Print("Group file has been written successfully to ", groupName)
	return nil
}

func dkgCmd(c *cli.Context) error {
	fs := NewFileStore(c)
	drand, err := LoadDrand(fs)
	if err != nil {
		slog.Fatal("could not load drand: ", err)
	}
	if c.Bool("leader") {
		return drand.StartDKG()
	} else {
		return drand.RunDKG()
	}
}

func beaconCmd(c *cli.Context) error {
	fs := NewFileStore(c)
	drand, err := LoadDrand(fs)
	if err != nil {
		slog.Fatal("could not load drand: ", err)
	}
	if c.Bool("leader") {
		drand.RandomBeacon([]byte(c.String("seed")), c.Duration("period"))
	} else {
		drand.Loop()
	}
	return nil
}

func runCmd(c *cli.Context) error {
	dkgCmd(c)
	beaconCmd(c)
	return nil
}

func toArray(flags ...cli.Flag) []cli.Flag {
	return flags
}

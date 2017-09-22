package main

import (
	"fmt"
	"os"
	"time"

	"github.com/nikkolasg/slog"
	"github.com/urfave/cli"
)

const version = "0.1"
const defaultSeed = "Expose yourself to your deepest fear; after that," +
	" fear has no power, and the fear of freedom shrinks and vanishes. " +
	" You are free. Morrisson"
const defaultPeriod = 30 * time.Minute

func banner() {
	fmt.Printf("drand v%s by nikkolasg @ DEDIS, EPFL\n", version)
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
			Name:    "dkg",
			Aliases: []string{"d"},
			Usage:   "Run the DKG protocol",
			Flags:   toArray(privFlag, groupFlag, shareFlag, leaderFlag),
			Action: func(c *cli.Context) error {
				fmt.Println("-> dkg")
				return nil
			},
		},
		cli.Command{
			Name:    "beacon",
			Aliases: []string{"b"},
			Usage:   "Run the beacon protocol",
			Flags: toArray(privFlag, groupFlag, shareFlag, sigFlag,
				leaderFlag, periodFlag, seedFlag),
			Action: func(c *cli.Context) error {
				fmt.Println("-> tbls")
				return nil
			},
		},
		cli.Command{
			Name:    "run",
			Aliases: []string{"r"},
			Usage:   "Run the daemon, first do the dkg then run the beacon",
			Flags: toArray(privFlag, groupFlag, shareFlag, sigFlag,
				leaderFlag, periodFlag, seedFlag),
			Action: func(c *cli.Context) error {
				fmt.Println(" -> daemon")
				return nil
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

package main

import (
	"errors"
	"fmt"
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
	s := "WARNING: this software has NOT received a full audit and therefore\n" +
		"WARNING: must be used with caution and NOT in a production environment.\n"
	fmt.Printf(s)
}

func main() {
	banner()
	app := cli.NewApp()
	app.Version = version
	// global flags re-used in many commands
	privFlag := cli.StringFlag{
		Name:  "private, p",
		Value: defaultPrivateFile(),
		Usage: "private key file path",
	}
	groupFlag := cli.StringFlag{
		Name:  "group, g",
		Value: defaultGroupFile(),
		Usage: "group file listing identities of participants",
	}
	shareFlag := cli.StringFlag{
		Name:  "share, s",
		Value: shareFile(defaultGroupFile()),
		Usage: "private share file path of the group",
	}
	sigFlag := cli.StringFlag{
		Name:  "beacon, b",
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
			Name:    "keygen",
			Aliases: []string{"k"},
			Flags:   toArray(privFlag),
			Usage:   "keygen <address to listen>. Generates longterm private key pair",
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
}

func keygenCmd(c *cli.Context) error {
	args := c.Args()
	if !args.Present() {
		return errors.New("no address present as argument")
	}
	if !isValidIP(args.First()) {
		return errors.New("address given is not a valid ip address")
	}
	priv := NewKeyPair(args.First())
	fs := NewFileStore(c)
	if err := fs.SaveKey(priv); err != nil {
		return err
	}
	slog.Info("Generated private key at ", fs.KeyFile)
	slog.Info("Generated public key at ", fs.PublicFile)
	return nil
}

func dkgCmd(c *cli.Context) error {
	fs := NewFileStore(c)
	drand, err := LoadDrand(fs)
	if err != nil {
		return err
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
		return err
	}
	if c.Bool("leader") {
		drand.RandomBeacon([]byte(c.String("seed")), c.Duration("period"))
	} else {
		drand.Loop()
	}
	return nil
}

func runCmd(c *cli.Context) error {
	if err := dkgCmd(c); err != nil {
		return err
	}
	return beaconCmd(c)
}

func toArray(flags ...cli.Flag) []cli.Flag {
	return flags
}

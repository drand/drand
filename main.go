package main

import (
	"errors"
	"fmt"

	"github.com/nikkolasg/slog"
	"github.com/urfave/cli"
)

const version = "0.1"

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

	app.Commands = []cli.Command{
		cli.Command{
			Name:    "keygen",
			Aliases: []string{"k"},
			Flags:   toArray(privFlag),
			Usage:   "keygen <address to listen>. Generates longterm private key pair",
			Action: func(c *cli.Context) error {
				return keygen(c)
			},
		},
		cli.Command{
			Name:    "dkg",
			Aliases: []string{"d"},
			Usage:   "Run the DKG protocol",
			Flags:   toArray(privFlag, groupFlag, shareFlag),
			Action: func(c *cli.Context) error {
				fmt.Println("-> dkg")
				return nil
			},
		},
		cli.Command{
			Name:    "beacon",
			Aliases: []string{"b"},
			Usage:   "Run the beacon protocol",
			Flags:   toArray(privFlag, groupFlag, shareFlag, sigFlag),
			Action: func(c *cli.Context) error {
				fmt.Println("-> tbls")
				return nil
			},
		},
		cli.Command{
			Name:    "run",
			Aliases: []string{"r"},
			Usage:   "Run the daemon, first do the dkg then run the beacon",
			Flags:   toArray(privFlag, groupFlag, shareFlag, sigFlag),
			Action: func(c *cli.Context) error {
				fmt.Println(" -> daemon")
				return nil
			},
		},
	}
}

func keygen(c *cli.Context) error {
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

func toArray(flags ...cli.Flag) []cli.Flag {
	return flags
}

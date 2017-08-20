package main

import "fmt"
import "github.com/urfave/cli"

const version = "0.1"

func banner() {
	fmt.Printf("drand v%s by nikkolasg and Philipp Jovanovic.\n", version)
	s := "WARNING: this software has NOT received a full audit and therefore\n" +
		"WARNING: must be used with caution and NOT in a production environment.\n"
	fmt.Printf(s)
}

func main() {
	banner()
	app := cli.NewApp()

	privFlag := cli.StringFlag{
		Name:  "private, p",
		Value: defaultPrivateFile(),
		Usage: "private key file path",
	}

	allFlags := []cli.Flag{privFlag}

	app.Commands = []cli.Command{
		cli.Command{
			Name:    "keygen",
			Aliases: []string{"k"},
			Usage:   "Generate longterm private key pair",
			Action: func(c *cli.Context) error {
				fmt.Println("-> keygen")
				return nil
			},
		},
		cli.Command{
			Name:    "dkg",
			Aliases: []string{"d"},
			Usage:   "Run the DKG protocol",
			Flags:   allFlags,
			Action: func(c *cli.Context) error {
				fmt.Println("-> dkg")
				return nil
			},
		},
		cli.Command{
			Name:    "tbls",
			Aliases: []string{"t"},
			Usage:   "Run the TBLS protocol",
			Flags:   allFlags,
			Action: func(c *cli.Context) error {
				fmt.Println("-> tbls")
				return nil
			},
		},
	}
}

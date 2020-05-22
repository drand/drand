package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/drand/drand/client"
	"github.com/urfave/cli/v2"
)

var urlFlag = &cli.StringFlag{
	Name:  "url",
	Usage: "root URL for fetching randomness",
}

var hashFlag = &cli.StringFlag{
	Name:  "hash",
	Usage: "The hash (in hex) for the chain to follow",
}

var insecureFlag = &cli.BoolFlag{
	Name:  "insecure",
	Usage: "Allow autodetection of the chain information",
}

var watchFlag = &cli.BoolFlag{
	Name:  "watch",
	Usage: "stream new values as they become available",
}

var roundFlag = &cli.IntFlag{
	Name:  "round",
	Usage: "request randomness for a specific round",
}

func main() {
	app := &cli.App{
		Name:   "client",
		Usage:  "CDN Drand client for loading randomness from an HTTP endpoint",
		Flags:  []cli.Flag{urlFlag, hashFlag, insecureFlag, watchFlag, roundFlag},
		Action: Client,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

// Client loads randomness from a server
func Client(c *cli.Context) error {
	if !c.IsSet(urlFlag.Name) {
		return fmt.Errorf("A URL is required to learn randomness from an HTTP endpoint")
	}

	opts := []client.Option{}

	if c.IsSet(hashFlag.Name) {
		hex, err := hex.DecodeString(c.String(hashFlag.Name))
		if err != nil {
			return err
		}
		opts = append(opts, client.WithChainHash(hex))
	}
	if c.IsSet(insecureFlag.Name) {
		opts = append(opts, client.WithInsecureHTTPEndpoints([]string{c.String(urlFlag.Name)}))
	} else {
		opts = append(opts, client.WithHTTPEndpoints([]string{c.String(urlFlag.Name)}))
	}
	client, err := client.New(opts...)
	if err != nil {
		return err
	}

	if c.IsSet(watchFlag.Name) {
		return Watch(c, client)
	}

	round := uint64(0)
	if c.IsSet(roundFlag.Name) {
		round = uint64(c.Int(roundFlag.Name))
	}
	rand, err := client.Get(context.Background(), round)
	if err != nil {
		return err
	}
	fmt.Printf("%v\n", rand)
	return nil
}

// Watch streams randomness from a client
func Watch(c *cli.Context, client client.Client) error {
	results := client.Watch(context.Background())
	for r := range results {
		fmt.Printf("%d\t%x\n", r.Round(), r.Randomness())
	}
	return nil
}

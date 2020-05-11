package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/drand/drand/client"
	"github.com/urfave/cli/v2"
)

var urlFlag = &cli.StringFlag{
	Name:  "url",
	Usage: "root URL for fetching randomness",
}

var keyFlag = &cli.StringFlag{
	Name:  "key",
	Usage: "The distributed key for the group to follow",
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
		Flags:  []cli.Flag{urlFlag, keyFlag, watchFlag, roundFlag},
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

	hashBytes := []byte{}
	if c.IsSet(keyFlag.Name) {
		hex, err := hex.DecodeString(c.String(keyFlag.Name))
		if err != nil {
			return err
		}
		hashBytes = hex
	}
	client, err := client.NewHTTPClient(c.String(urlFlag.Name), hashBytes, &http.Client{})
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
		fmt.Printf("%v\n", r)
	}
	return nil
}

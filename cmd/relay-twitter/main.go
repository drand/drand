package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path"
	"strings"
	"time"

	"github.com/drand/drand/client"
	"github.com/drand/drand/cmd/client/lib"
	"github.com/drand/drand/log"
	cli "github.com/urfave/cli/v2"

	"github.com/kurrik/oauth1a"
	"github.com/kurrik/twittergo"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.version=`git describe --tags`
//   -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	version   = "master"
	gitCommit = "none"
	buildDate = "unknown"
)

var (
	credsPathFlag = &cli.StringFlag{
		Name:  "creds",
		Usage: "Location of credentials file, newline separated API key, API secret, access token, access token secret (default: ~/.twitter/CREDENTIALS)",
	}
)

func main() {
	app := &cli.App{
		Name:     "drand-relay-twitter",
		Version:  version,
		Usage:    "Twitter relay for randomness beacon",
		Commands: []*cli.Command{runCmd},
	}
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("drand Twitter relay %v (date %v, commit %v)\n", version, buildDate, gitCommit)
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("error: %+v\n", err)
		os.Exit(1)
	}
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "start a drand Twitter relay process",
	Flags: append(lib.ClientFlags, credsPathFlag),

	Action: func(cctx *cli.Context) error {
		credsPath := cctx.String(credsPathFlag.Name)
		if credsPath == "" {
			usr, err := user.Current()
			if err != nil {
				return fmt.Errorf("getting current user: %w", err)
			}
			credsPath = path.Join(usr.HomeDir, ".twitter", "CREDENTIALS")
		}

		config, user, err := loadCredentials(credsPath)
		twc := twittergo.NewClient(config, user)

		c, err := lib.Create(cctx, false)
		if err != nil {
			return fmt.Errorf("creating client: %w", err)
		}

		watch(context.Background(), c, twc)
		return nil
	},
}

func loadCredentials(path string) (*oauth1a.ClientConfig, *oauth1a.UserConfig, error) {
	credentials, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	lines := strings.Split(string(credentials), "\n")
	fmt.Printf("'%s', '%s', '%s', '%s'\n", lines[0], lines[1], lines[2], lines[3])
	config := &oauth1a.ClientConfig{
		ConsumerKey:    lines[0],
		ConsumerSecret: lines[1],
	}
	user := oauth1a.NewAuthorizedConfig(lines[2], lines[3])
	return config, user, nil
}

func watch(ctx context.Context, c client.Watcher, twc *twittergo.Client) {
	for {
		ch := c.Watch(ctx)
	INNER:
		for {
			select {
			case res, ok := <-ch:
				if !ok {
					log.DefaultLogger().Warn("relay_twitter", "watch channel closed")
					t := time.NewTimer(time.Second)
					select {
					case <-t.C:
						break INNER
					case <-ctx.Done():
						return
					}
				}
				log.DefaultLogger().Info("relay_twitter", "got randomness", "round", res.Round())
				go func(res client.Result) {
					url, err := tweetRandomness(ctx, twc, res)
					if err != nil {
						log.DefaultLogger().Error("relay_twitter", "failed to tweet randomness", "err", err)
						return
					}
					log.DefaultLogger().Info("relay_twitter", "tweeted randomness", "round", res.Round(), "location", url)
				}(res)
			case <-ctx.Done():
				return
			}
		}
	}
}

func tweetRandomness(ctx context.Context, twc *twittergo.Client, res client.Result) (string, error) {
	data := url.Values{}
	data.Set("status", hex.EncodeToString(res.Randomness()))
	body := strings.NewReader(data.Encode())
	req, err := http.NewRequest("POST", "/1.1/statuses/update.json", body)
	if err != nil {
		return "", fmt.Errorf("parsing request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := twc.SendRequest(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	tweet := &twittergo.Tweet{}
	err = resp.Parse(tweet)
	if err != nil {
		if rle, ok := err.(twittergo.RateLimitError); ok {
			return "", fmt.Errorf("rate limited, reset at %v: %w", rle.Reset, err)
		}
		return "", fmt.Errorf("parsing response: %w", err)
	}
	return fmt.Sprintf("https://twitter.com/%s/status/%s", tweet.User().ScreenName(), tweet.IdStr()), nil
}

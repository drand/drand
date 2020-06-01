package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/push"

	"github.com/drand/drand/client"
	gclient "github.com/drand/drand/cmd/relay-gossip/client"
	"github.com/drand/drand/cmd/relay-gossip/lp2p"
	"github.com/drand/drand/cmd/relay-gossip/node"
	dlog "github.com/drand/drand/log"
	bds "github.com/ipfs/go-ds-badger2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`"
var (
	version   = "master"
	gitCommit = "none"
	buildDate = "unknown"
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

var relayPeersFlag = &cli.StringSliceFlag{
	Name:  "relays",
	Usage: "list of multiaddresses of relay peers to connect with",
}

var relayPortFlag = &cli.IntFlag{
	Name:  "port",
	Usage: "port for client's peer host, when connecting to relays",
}

// client metric flags

var clientMetricsAddressFlag = &cli.StringFlag{
	Name:  "client-metrics-address",
	Usage: "Server address for Prometheus metrics.",
	Value: ":8080",
}

var clientMetricsGatewayFlag = &cli.StringFlag{
	Name:  "client-metrics-gateway",
	Usage: "Push gateway for Prometheus metrics.",
}

var clientMetricsPushIntervalFlag = &cli.Int64Flag{
	Name:  "client-metrics-push-interval",
	Usage: "Push interval in seconds for Prometheus gateway.",
}

var clientMetricsIDFlag = &cli.StringFlag{
	Name:  "client-metrics-id",
	Usage: "Unique identifier for the client instance, used by the metrics system.",
}

func main() {
	app := cli.NewApp()
	app.Name = "demo-client"
	app.Version = version
	app.Usage = "CDN Drand client for loading randomness from an HTTP endpoint"
	app.Flags = []cli.Flag{
		urlFlag, hashFlag, insecureFlag, watchFlag, roundFlag,
		relayPeersFlag, relayPortFlag,
		clientMetricsAddressFlag, clientMetricsGatewayFlag, clientMetricsIDFlag,
		clientMetricsPushIntervalFlag,
	}
	app.Action = Client
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("drand client %v (date %v, commit %v)\n", version, buildDate, gitCommit)
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

	if c.IsSet(relayPeersFlag.Name) {
		relayPeers, err := node.ParseMultiaddrSlice(c.StringSlice(relayPeersFlag.Name))
		if err != nil {
			return err
		}
		ps, err := buildClientHost(c.Int(relayPortFlag.Name), relayPeers)
		if err != nil {
			return err
		}
		opts = append(opts, gclient.WithPubsub(ps))
	}

	if c.IsSet(clientMetricsIDFlag.Name) {
		clientID := c.String(clientMetricsIDFlag.Name)
		if !c.IsSet(clientMetricsAddressFlag.Name) && !c.IsSet(clientMetricsGatewayFlag.Name) {
			return fmt.Errorf("missing prometheus address or push gateway")
		}
		metricsAddr := c.String(clientMetricsAddressFlag.Name)
		metricsGateway := c.String(clientMetricsGatewayFlag.Name)
		metricsPushInterval := c.Int64(clientMetricsPushIntervalFlag.Name)
		bridge := newPrometheusBridge(metricsAddr, metricsGateway, metricsPushInterval)
		bridgeWithID := client.WithPrometheus(prometheus.WrapRegistererWith(
			prometheus.Labels{"client_id": clientID},
			bridge))
		opts = append(opts, bridgeWithID)
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

func buildClientHost(clientRelayPort int, relayMultiaddr []ma.Multiaddr) (*pubsub.PubSub, error) {
	clientID := uuid.New().String()
	ds, err := bds.NewDatastore(path.Join(os.TempDir(), "drand-client-"+clientID+"-datastore"), nil)
	if err != nil {
		return nil, err
	}
	priv, err := lp2p.LoadOrCreatePrivKey(path.Join(os.TempDir(), "drand-client-"+clientID+"-id"), dlog.DefaultLogger)
	if err != nil {
		return nil, err
	}
	_, ps, err := lp2p.ConstructHost(
		ds,
		priv,
		"/ip4/0.0.0.0/tcp/"+strconv.Itoa(clientRelayPort),
		relayMultiaddr,
		dlog.DefaultLogger,
	)
	if err != nil {
		return nil, err
	}
	return ps, nil
}

// Watch streams randomness from a client
func Watch(c *cli.Context, client client.Client) error {
	results := client.Watch(context.Background())
	for r := range results {
		fmt.Printf("%d\t%x\n", r.Round(), r.Randomness())
	}
	return nil
}

func newPrometheusBridge(address string, gateway string, pushIntervalSec int64) prometheus.Registerer {
	b := &prometheusBridge{
		address:         address,
		pushIntervalSec: pushIntervalSec,
		Registry:        prometheus.NewRegistry(),
	}
	if gateway != "" {
		b.pusher = push.New(gateway, "drand_client_observations_push").Gatherer(b.Registry)
		go b.pushLoop()
	}
	if address != "" {
		http.Handle("/metrics", promhttp.HandlerFor(b.Registry, promhttp.HandlerOpts{
			Timeout: 10 * time.Second,
		}))
		go func() {
			log.Fatal(http.ListenAndServe(address, nil))
		}()
	}
	return b
}

type prometheusBridge struct {
	*prometheus.Registry
	address         string
	pushIntervalSec int64
	pusher          *push.Pusher
}

func (b *prometheusBridge) pushLoop() {
	for {
		time.Sleep(time.Second * time.Duration(b.pushIntervalSec))
		if err := b.pusher.Push(); err != nil {
			log.Printf("prometheus gateway push (%v)", err)
		}
	}
}

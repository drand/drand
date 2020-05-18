package client

import (
	"context"
	"fmt"
	"time"

	"github.com/drand/drand/beacon"
	dclient "github.com/drand/drand/client"
	"github.com/drand/drand/cmd/relay-gossip/lp2p"
	"github.com/drand/drand/key"
	dlog "github.com/drand/drand/log"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"golang.org/x/xerrors"
)

// ErrNotAvailable is returned when Get is called using a basic client with no HTTP API configuration.
var ErrNotAvailable = fmt.Errorf("not available")

// Options are configuration options for the drand libp2p pubsub client
type Options struct {
	// HTTPEndpoints are drand HTTP API URL(s) to use incase of gossipsub failure.
	HTTPEndpoints []string
	// FailoverGracePeriod is the period before the failover HTTP API is used when watching for randomness.
	FailoverGracePeriod time.Duration
	// Logger is a custom logger to use with the client
	Logger dlog.Logger
}

type basicClient struct {
	group       *key.Group
	getNotifier dclient.GetNotifierFunc
}

// NewWithPubsub creates a new drand client that uses pubsub to receive randomness updates.
func NewWithPubsub(ps *pubsub.PubSub, group *key.Group, options Options) (dclient.Client, error) {
	t, err := ps.Join(lp2p.PubSubTopic(string(group.Hash())))
	if err != nil {
		return nil, xerrors.Errorf("joining pubsub: %w", err)
	}

	log := options.Logger
	if log == nil {
		log = dlog.DefaultLogger
	}

	if len(options.HTTPEndpoints) == 0 {
		return &basicClient{group: group, getNotifier: NewNotifier(t, log)}, nil
	}

	return dclient.New(
		dclient.WithGroup(group),
		dclient.WithHTTPEndpoints(options.HTTPEndpoints),
		dclient.WithGetNotifierFunc(NewFailoverNotifier(t, options.FailoverGracePeriod, log)),
	)
}

// Get returns a the randomness at `round` or an error.
func (c *basicClient) Get(ctx context.Context, round uint64) (dclient.Result, error) {
	return nil, ErrNotAvailable
}

// Watch returns new randomness as it becomes available.
func (c *basicClient) Watch(ctx context.Context) <-chan dclient.Result {
	return c.getNotifier(ctx, c, c.group)
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (c *basicClient) RoundAt(time time.Time) uint64 {
	return beacon.CurrentRound(time.Unix(), c.group.Period, c.group.GenesisTime)
}

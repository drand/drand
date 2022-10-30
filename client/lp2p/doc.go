/*
Package lp2p provides a drand client implementation that retrieves
randomness by subscribing to a libp2p pubsub topic.

WARNING: this client can only be used to "Watch" for new randomness rounds and
"Get" randomness rounds it has previously seen that are still in the cache.

If you need to "Get" arbitrary rounds from the chain then you must combine this client with the http or grpc clients.

The agnostic client builder must receive "WithChainInfo()" in order for it to
validate randomness rounds it receives, or "WithChainHash()" and be combined
with the HTTP or gRPC client implementations so that chain information can be
fetched from them.

It is particularly important that rounds are verified since they can be delivered by any peer in the network.

Example using "WithChainInfo()":

	package main

	import (
		"context"

		clock "github.com/jonboulle/clockwork"
		pubsub "github.com/libp2p/go-libp2p-pubsub"

		"github.com/drand/drand/client"
		gclient "github.com/drand/drand/client/lp2p"
		"github.com/drand/drand/common/chain"
		"github.com/drand/drand/common/log"
	)

	func main() {
		ctx := context.Background()
		lg := log.New(nil, log.DebugLevel, true)
		clk := clock.NewRealClock()

		ps := newPubSub()
		info := readChainInfo()

		c, err := client.New(ctx, lg,
			gclient.WithPubsub(lg, ps, clk, gclient.DefaultBufferSize),
			client.WithChainInfo(info),
		)
	}

	func newPubSub() *pubsub.Pubsub {
		// ...
	}

	func readChainInfo() *chain.Info {
		// ...
	}

Example using "WithChainHash()" and combining it with a different client:

	package main

	import (
		"context"
		"encoding/hex"

		clock "github.com/jonboulle/clockwork"
		pubsub "github.com/libp2p/go-libp2p-pubsub"

		"github.com/drand/drand/client"
		"github.com/drand/drand/client/http"
		gclient "github.com/drand/drand/client/lp2p"
		"github.com/drand/drand/common/log"
	)

	var urls = []string{
		"https://api.drand.sh",
		"https://drand.cloudflare.com",
		// ...
	}

	var chainHash, _ = hex.DecodeString("8990e7a9aaed2ffed73dbd7092123d6f289930540d7651336225dc172e51b2ce")

	func main() {
		ctx := context.Background()
		lg := log.New(nil, log.DebugLevel, true)
		clk := clock.NewRealClock()
		ps := newPubSub()

		c, err := client.New(ctx, lg,
			gclient.WithPubsub(lg, ps, clk, gclient.DefaultBufferSize),
			client.WithChainHash(chainHash),
			client.From(http.ForURLs(ctx, lg, urls, chainHash)...),
		)
	}

	func newPubSub() *pubsub.Pubsub {
		// ...
	}
*/
package lp2p

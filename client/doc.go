/*
Package client provides transport-agnostic logic to retrieve and verify
randomness from drand, including retry, validation, caching and
optimization features.

Example:

	import (
		"context"
		"encoding/hex"
		"fmt"

		"github.com/drand/drand/client"
	)

	var chainHash, _ = hex.DecodeString("8990e7a9aaed2ffed73dbd7092123d6f289930540d7651336225dc172e51b2ce")

	func main() {
		c, err := client.New(
			client.From("..."), // see concrete client implementations
			client.WithChainHash(chainHash),
		)

		// e.g. use the client to get the latest randomness round:
		r := c.Get(context.Background(), 0)

		fmt.Println(r.Round(), r.Randomness())
	}

The "From" option allows you to specify clients that work over particular
transports. HTTP, gRPC and libp2p PubSub clients are provided in drand's
subpackages https://pkg.go.dev/github.com/drand/drand/client/http,
https://pkg.go.dev/github.com/drand/drand/client/grpc and
https://pkg.go.dev/github.com/drand/drand/lp2p/clientlp2p/client
respectively. Note that you are not restricted to just one client. You can use
multiple clients of the same type or of different types. The base client will
periodically "speed test" it's clients, failover, cache results and aggregate
calls to "Watch" to reduce requests.

WARNING: When using the client you should use the "WithChainHash" or
"WithChainInfo" option in order for your client to validate the randomness it
receives is from the correct chain. You may use the "Insecurely" option to
bypass this validation but it is not recommended.

In an application that uses the drand client, the following options are likely
to be needed/customized:

	WithCacheSize()
		should be set to something sensible for your application.

	WithVerifiedResult()
	WithFullChainVerification()
		both should be set for increased security if you have
		persistent state and expect to be following the chain.

	WithAutoWatch()
		will pre-load new results as they become available adding them
		to the cache for speedy retreival when you need them.

	WithPrometheus()
		enables metrics reporting on speed and performance to a
		provided prometheus registry.

*/
package client

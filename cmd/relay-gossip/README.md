<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
## Table of Contents

- [Drand Pubsub Relay](#drand-pubsub-relay)
  - [Install](#install)
  - [Usage](#usage)
    - [Relay gRPC](#relay-grpc)
    - [Relay HTTP](#relay-http)
    - [Relay Gossipsub](#relay-gossipsub)
    - [Other options](#other-options)
      - [Bootstrap peers](#bootstrap-peers)
      - [Failover](#failover)
      - [Configuring the libp2p pubsub node](#configuring-the-libp2p-pubsub-node)
    - [Usage from a golang drand client](#usage-from-a-golang-drand-client)
      - [With Group TOML or Chain Info](#with-group-toml-or-chain-info)
      - [With Known Chain Hash](#with-known-chain-hash)
      - [Insecurely](#insecurely)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Drand Pubsub Relay

A program that relays drand randomness rounds over libp2p pubsub (gossipsub) from a gRPC, HTTP, or gossipsub source (labeled as _drand gossipsub relay_ in this diagram):

```
              +-------------------------------+
              |                               |
              |         drand server          |
              |                               |
              +-------------------------------+
              |  gRPC API   |--|   HTTP API   |
              +------^-+------------^-+-------+
                     | |            | |
                     | |            | |
                     | |            | |
                  +--+-v------------+-v---+
                  | drand gossipsub relay |
                  +----------+------------+
                             |
                             |
Publish topic=/drand/pubsub/v0.0.0/<chain-hash> data={randomness}
                             |
                             |
                 +-----------v--------------+
                 |                          |
                 | libp2p gossipsub network |
                 |                          |
                 +--+--------------------+--+
                    |                    |
                    |                    |
        Subscribe topic=/drand/pubsub/v0.0.0/<chain-hash>
                    |                    |
                    |                    |
   +----------------v--------+   +-------v-----------------+
   | drand client WithPubsub |   | drand client WithPubsub |
   +-------------------------+   +-------------------------+
```

## Install

```sh
# Clone this repo
git clone https://github.com/drand/drand.git
cd drand
# Build the executable
make relay-gossip
# Outputs a `drand-relay-gossip` executable to the current directory.
```

## Usage

In general, you _should_ specify either a `-hash` or `-group-conf` flag in order for your client to validate the randomness it receives is from the correct chain.

### Relay gRPC

```sh
drand-relay-gossip run -grpc-connect=127.0.0.1:3000 \
                       -cert=/path/to/grpc-drand-cert
```

If you do not have gRPC transport credentials, you can use the `-insecure` flag:

```sh
drand-relay-gossip run -grpc-connect=127.0.0.1:3000 \
                       -insecure
```

### Relay HTTP

The gossip relay can also relay directly from an HTTP API. You can specify multiple endpoints to enable failover.

```sh
drand-relay-gossip run -url=http://127.0.0.1:3002 \
                       -url=http://127.0.0.1:3102 \
                       -hash=6093f9e4320c285ac4aab50ba821cd5678ec7c5015d3d9d11ef89e2a99741e83
```

### Relay Gossipsub

The gossip relay can also relay directly from _other_ gossip relays. You can specify multiple peers to directly connect with. In this case, a group configuration file must be specified since there's no way to retrieve chain information over pubsub.

```sh
drand-relay-gossip run -relay=/ip4/127.0.0.1/tcp/44544/p2p/QmPeerID0 \
                       -relay=/ip4/127.0.0.1/tcp/44545/p2p/QmPeerID1 \
                       -group-conf=/home/user/.drand/groups/drand_group.toml
```

Alternatively, you can provide URL(s) of HTTP API(s) that can be contacted to retrieve chain information. In this case we must provide the chain `-hash` to verify the information we retrieve is for the chain we expect (or provide the `-insecure` flag):

```sh
drand-relay-gossip run -relay=/ip4/127.0.0.1/tcp/44544/p2p/QmPeerID0 \
                       -relay=/ip4/127.0.0.1/tcp/44545/p2p/QmPeerID1 \
                       -url=http://127.0.0.1:3002 \
                       -hash=6093f9e4320c285ac4aab50ba821cd5678ec7c5015d3d9d11ef89e2a99741e83
```

### Other options

#### Bootstrap peers

If there is a set of peers the gossip relay should connect with and stay connected to then the `-peer-with` flag can be used to specify one or more peer multiaddrs for this purpose.

#### Failover

The `-url` flag provides the URL(s) of alternative HTTP API endpoints that may be able to provide randomness in the event of a failure of the gRPC connection/libp2p pubsub network. Each randomness round is raced with the HTTP endpoints when it becomes available such that if gRPC or pubsub take too long to deliver the round it'll be provided over HTTP e.g.

```sh
drand-relay-gossip run -grpc-connect=127.0.0.1:3000 \
                       -insecure \
                       -url=http://127.0.0.1:3102
```

```sh
drand-relay-gossip run -relay=/ip4/127.0.0.1/tcp/44544/p2p/QmPeerID0 \
                       -relay=/ip4/127.0.0.1/tcp/44545/p2p/QmPeerID1 \
                       -hash=6093f9e4320c285ac4aab50ba821cd5678ec7c5015d3d9d11ef89e2a99741e83 \
                       -url=http://127.0.0.1:3102
```

#### Configuring the libp2p pubsub node

Starting a relay will spawn a libp2p pubsub node listening on `/ip4/0.0.0.0/tcp/44544` by default. Use the `-listen` flag to change. To effectively relay drand randomness, your node must be publicly accessible on the network.

If not specified a libp2p identity will be generated and stored in an `identity.key` file in the current working directory. Use the `-identity` flag to override the location.

### Usage from a golang drand client

#### With Group TOML or Chain Info

```go
package main

import (
	"context"
	"fmt"

	"github.com/alanshaw/drand-gossipsub-client-demo/util"
	"github.com/drand/drand/client"
	gclient "github.com/drand/drand/lp2p/client"
)

const (
   // listenAddr is the multiaddr the local libp2p node should listen on.
   listenAddr   = "/ip4/0.0.0.0/tcp/4453"
   // relayP2PAddr is the p2p multiaddr of the drand gossipsub relay node to connect to.
   relayP2PAddr = "/ip4/192.168.1.124/tcp/44544/p2p/QmPeerID"
   // groupTOMLPath is the path to the group configuration information (in TOML format).
   groupTOMLPath = "/home/user/.drand/groups/drand_group.toml"
)

func main() {
	// Create libp2p pubsub
	ps := util.NewPubsub(listenAddr, relayP2PAddr)

	// Extract chain info from group TOML
	info := util.ChainInfoFromGroupTOML(groupTOMLPath)

	c, err := client.New(gclient.WithPubsub(ps), client.WithChainInfo(info))
	if err != nil {
		panic(err)
	}

	for res := range c.Watch(context.Background()) {
		fmt.Printf("round=%v randomness=%v\n", res.Round(), res.Randomness())
	}
}
```

#### With Known Chain Hash

You do not need to know the full group info to use the pubsub client if you know the chain hash and an HTTP endpoint then you can request the chain info from the HTTP endpoint, verifying it with the known chain hash:

```go
package main

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/alanshaw/drand-gossipsub-client-demo/util"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/http"
	gclient "github.com/drand/drand/lp2p/client"
)

const (
   // listenAddr is the multiaddr the local libp2p node should listen on.
   listenAddr   = "/ip4/0.0.0.0/tcp/4453"
   // relayP2PAddr is the p2p multiaddr of the drand gossipsub relay node to connect to.
   relayP2PAddr = "/ip4/192.168.1.124/tcp/44544/p2p/12D3KooWAe637xuWdRCYkuaZZce13P1F9zJX5gzGUPWZJpsUGUSH"
   // chainHash is a hash of the group chain information.
   chainHash    = "c599c267a0dd386606f7d6132da8327d57e1004760897c9dd4fb8495c29942b2"
   // httpRelayURL is the URL of a drand HTTP API endpoint.
   httpRelayURL = "http://127.0.0.1:3002"
)

func main() {
	// Create libp2p pubsub
	ps := util.NewPubsub(listenAddr, relayP2PAddr)

	// Chain hash is used to verify endpoints
	hash, err := hex.DecodeString(chainHash)
	if err != nil {
		panic(err)
	}

	c, err := client.New(
		gclient.WithPubsub(ps),
		client.WithChainHash(hash),
		client.From(http.ForURLs([]string{httpRelayURL}, hash)...),
	)
	if err != nil {
		panic(err)
	}

	for res := range c.Watch(context.Background()) {
		fmt.Printf("round=%v randomness=%v\n", res.Round(), res.Randomness())
	}
}
```

#### Insecurely

If you trust the HTTP(S) endpoint, you don't need chain info or a chain hash. Note: using HTTP**S** provides trust at the transport level, but it does not allow verification that the randomness is being requested from the expected chain:

```go
package main

import (
	"context"
	"fmt"

	"github.com/alanshaw/drand-gossipsub-client-demo/util"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/http"
	gclient "github.com/drand/drand/lp2p/client"
)

const (
   // listenAddr is the multiaddr the local libp2p node should listen on.
   listenAddr   = "/ip4/0.0.0.0/tcp/4453"
   // relayP2PAddr is the p2p multiaddr of the drand gossipsub relay node to connect to.
   relayP2PAddr = "/ip4/192.168.1.124/tcp/44544/p2p/QmPeerID"
   // httpRelayURL is the URL of a drand HTTP API endpoint.
   httpRelayURL = "http://127.0.0.1:3002"
)

func main() {
	ps := util.NewPubsub(listenAddr, relayP2PAddr)

	c, err := client.New(
		gclient.WithPubsub(ps),
		client.From(http.ForURLs([]string{httpRelayURL}, nil)...),
		client.Insecurely(),
	)
	if err != nil {
		panic(err)
	}

	for res := range c.Watch(context.Background()) {
		fmt.Printf("round=%v randomness=%v\n", res.Round(), res.Randomness())
	}
}
```

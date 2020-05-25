module github.com/drand/drand/cmd/relay-gossip

go 1.14

replace github.com/drand/drand => ../../

require (
	cloud.google.com/go/pubsub v1.2.0
	github.com/drand/drand v0.8.2-0.20200508124210-33866c2232e3
	github.com/drand/drand/cmd/drand-gossip-relay v0.0.0-20200515153120-57a6056a24d4
	github.com/gogo/protobuf v1.3.1
	github.com/googleapis/gax-go v1.0.3 // indirect
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-ds-badger2 v0.1.0
	github.com/ipfs/go-log/v2 v2.0.8
	github.com/libp2p/go-libp2p v0.9.2
	github.com/libp2p/go-libp2p-core v0.5.6
	github.com/libp2p/go-libp2p-peerstore v0.2.4
	github.com/libp2p/go-libp2p-pubsub v0.3.0
	github.com/libp2p/go-libp2p-tls v0.1.3
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/prometheus/common v0.9.1
	github.com/urfave/cli/v2 v2.2.0
	golang.org/x/crypto v0.0.0-20200510223506-06a226fb4e37
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
	google.golang.org/api v0.25.0 // indirect
	google.golang.org/grpc v1.28.0
)

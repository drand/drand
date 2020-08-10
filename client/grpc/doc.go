/*
Package grpc provides a drand client implementation that uses drand's gRPC API.

The client connects to a drand gRPC endpoint to fetch randomness. The gRPC
client has some advantages over the HTTP client - it is more compact
on-the-wire and supports streaming and authentication.

Example:

	package main

	import (
		"encoding/hex"

		"github.com/drand/drand/client"
		"github.com/drand/drand/client/grpc"
	)

	const (
		grpcAddr = "example.drand.grpc.server:4444"
		certPath = "/path/to/drand-grpc.cert"
	)

	var chainHash, _ = hex.DecodeString("8990e7a9aaed2ffed73dbd7092123d6f289930540d7651336225dc172e51b2ce")

	func main() {
		gc, err := grpc.New(grpcAddr, certPath, false)

		c, err := client.New(
			client.From(gc),
			client.WithChainHash(chainHash),
		)
	}

A path to a file that holds TLS credentials for the drand server is required
to validate server connections. Alternatively set the final parameter to
`true` to enable _insecure_ connections (not recommended).
*/
package grpc

package net

import "strings"

// ADDRESSES and TLS
// https://github.com/denji/golang-tls
// How do we manage plain tcp and TLS connection with self-signed certificates
// or CA-signed certificates:
// (A) For non tls servers, when initiating connection just call with
// grpc.WithInsecure(). Same for listening ( see Golang gRPC API).
// (B) TLS communication using certificates
// How to differentiate (A) and (B) ?
// 	=> simple set of rules ? (xxx:443 | https | tls) == (B), rest is (A)
//
// For (B):
// Certificates is signed by a CA, so no options needed, simply
// 		crendentials.FromTLSCOnfig(&tls.Config{}) when connecting, or
// 		credentials.FromTLSConfig{&tls.Config{cert,private...}} for listening
// Certificates are given as a command line option "-cert xxx.crt" == (2) , otherwise (1)
// Since gRPC golang library does not allow us to access internal connections,
// every pair of communicating nodes is gonna have two active connections at the
// same time, one outgoing from each party.

// Peer is a simple interface that allows retrieving the address of a
// destination. It might further e enhanced with certificates properties and
// all.
type Peer interface {
	Address() string
}

// IsTLS returns true if the address starts with HTTPS
func IsTLS(addr string) bool {
	return strings.HasPrefix(addr, "https")
}

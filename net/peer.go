package net

import (
	"context"
	"strings"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// Peer is a simple interface that allows retrieving the address of a
// destination. It might further be enhanced with certificates properties and
// all.
type Peer interface {
	Address() string
	IsTLS() bool
}

type sPeer struct {
	addr string
	tls  bool
}

func (s *sPeer) Address() string {
	return s.addr
}

func (s *sPeer) IsTLS() bool {
	return s.tls
}

// CreatePeer creates a peer from an address
func CreatePeer(addr string, tls bool) Peer {
	return &sPeer{
		addr: addr,
		tls:  tls,
	}
}

// RemoteAddress returns the address of the peer by first taking the address
// that gRPC returns. If that address is a reserved address, then it tries to
// read the "X-REAL-IP" header content.
// For example, a valid nging config could include
//
// ```
// location / {
//       grpc_pass grpc://127.0.0.1:9091;
//       grpc_set_header X-Real-IP $remote_addr;
// }
// ```
//
func RemoteAddress(c context.Context) string {
	p, ok := peer.FromContext(c)
	var str string = ""
	if ok {
		str = p.Addr.String()
	}
	// https://en.wikipedia.org/wiki/Reserved_IP_addresses
	reserved := []string{
		"10.",
		"127.",
		"192.168",
		"100.64.",
		"172.16.",
		"169.254.",
	}

	var lookAtHeader bool
	if str == "" {
		lookAtHeader = true
	} else {
		for _, r := range reserved {
			if strings.HasPrefix(str, r) {
				lookAtHeader = true
			}
		}
	}

	if !lookAtHeader {
		return str
	}

	md, ok := metadata.FromIncomingContext(c)
	if !ok {
		return str
	}
	vals := md.Get("x-real-ip")
	if len(vals) > 0 {
		return vals[0]
	}
	return str
}

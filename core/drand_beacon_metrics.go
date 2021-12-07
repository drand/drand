package core

import (
	"context"
	"errors"
	"net/http"

	"github.com/drand/drand/net"
)

// PeerMetrics exports a handler for retreiving metric information from group peers
func (bp *BeaconProcess) PeerMetrics(c context.Context) (map[string]http.Handler, error) {
	if bp.group == nil {
		return nil, errors.New("no group yet")
	}

	pc := bp.privGateway.ProtocolClient
	hc, ok := pc.(net.HTTPClient)
	if !ok {
		return nil, errors.New("implementation does not support metrics")
	}

	handlers := make(map[string]http.Handler)
	var err error
	for _, n := range bp.group.Nodes {
		p := net.CreatePeer(n.Address(), n.IsTLS())
		if h, e := hc.HandleHTTP(p); e == nil {
			handlers[n.Address()] = h
		} else {
			err = e
		}
	}
	return handlers, err
}

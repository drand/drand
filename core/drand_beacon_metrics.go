package core

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
)

// GroupMetrics exports a handler for retrieving metric information from group peers
// It's a function so that it can be called lazily when nodes join/leave (e.g. during
// reshares)
func (bp *BeaconProcess) GroupMetrics() (map[string]http.Handler, error) {
	if bp.group == nil {
		return nil, fmt.Errorf("no group yet for beacon %s", bp.beaconID)
	}

	pc := bp.privGateway.ProtocolClient
	hc, ok := pc.(net.HTTPClient)
	if !ok {
		return nil, errors.New("the ProtocolClient implementation does not support metrics")
	}

	handlers := make(map[string]http.Handler)
	var err error
	for _, n := range bp.group.Nodes {
		log.DefaultLogger().Infow("", "metrics", "adding node to metrics", "beaconID", bp.beaconID, "address", n.Address())
		p := net.CreatePeer(n.Address(), n.IsTLS())
		if h, e := hc.HandleHTTP(p); e == nil {
			handlers[n.Address()] = h
		} else {
			err = e
		}
	}
	return handlers, err
}

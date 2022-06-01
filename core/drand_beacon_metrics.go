package core

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/drand/drand/net"
)

// GroupMetrics exports a map of handlers for retrieving metric information from group peers,
// keyed by each peer's address. It's a function so that it can be called lazily to take into
// account runtime changes in the group (e.g. during reshares)
func (bp *BeaconProcess) GroupMetrics() (map[string]http.Handler, error) {
	if bp.group == nil {
		return nil, fmt.Errorf("no group yet for beacon %s", bp.getBeaconID())
	}

	pc := bp.privGateway.ProtocolClient
	hc, ok := pc.(net.HTTPClient)
	if !ok {
		return nil, errors.New("the ProtocolClient implementation does not support metrics")
	}

	handlers := make(map[string]http.Handler)
	var err error
	for _, n := range bp.group.Nodes {
		bp.log.Debugw("", "metrics", "adding node to metrics", "address", n.Address())
		p := net.CreatePeer(n.Address(), n.IsTLS())
		if h, e := hc.HandleHTTP(p); e == nil {
			handlers[n.Address()] = h
		} else {
			bp.log.Infow("", "metrics", "Error while adding node", "address", n.Address(), "error", err)
			err = e
		}
	}

	return handlers, err
}

package core

import (
	"errors"
	"net/http"

	"github.com/drand/drand/common"
	"github.com/drand/drand/net"
)

// MetricsHandlerForPeer returns a handler for retrieving metric information from a peer in this group
func (bp *BeaconProcess) MetricsHandlerForPeer(addr string) (http.Handler, error) {
	if bp.group == nil {
		return nil, &common.NotPartOfGroup{BeaconID: bp.getBeaconID()}
	}

	pc := bp.privGateway.ProtocolClient
	hc, ok := pc.(net.HTTPClient)
	if !ok {
		return nil, errors.New("the ProtocolClient implementation does not support metrics")
	}

	var err error

	for _, n := range bp.group.Nodes {
		if n.Address() == addr {
			p := net.CreatePeer(n.Address(), n.IsTLS())
			if h, e := hc.HandleHTTP(p); e == nil {
				return h, nil
			} else {
				bp.log.Infow("", "metrics", "Error while adding node", "address", n.Address(), "error", err)
				err = e
			}
		}
	}

	return nil, err
}

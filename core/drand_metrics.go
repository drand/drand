package core

import (
	"context"
	"errors"
	"net/http"

	"github.com/drand/drand/net"
)

// PeerMetrics exports a handler for retreiving metric information from group peers
func (d *Drand) PeerMetrics(c context.Context) ([]http.Handler, error) {
	if d.group == nil {
		return nil, errors.New("no group yet")
	}

	pc := d.privGateway.ProtocolClient
	hc, ok := pc.(net.HTTPClient)
	if !ok {
		return nil, errors.New("Implementation does not support metrics")
	}

	handlers := make([]http.Handler, 0, len(d.group.Nodes))
	var err error
	for _, n := range d.group.Nodes {
		if n.Index == uint32(d.index) {
			continue
		}
		p := net.CreatePeer(n.Address(), n.IsTLS())
		if h, e := hc.HandleHTTP(p); err == nil {
			handlers = append(handlers, h)
		} else {
			err = e
		}
	}
	return handlers, err
}

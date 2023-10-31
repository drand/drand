package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/drand/drand/common"
	"github.com/drand/drand/common/tracer"
	"github.com/drand/drand/internal/net"
)

// HTTPClient is currently used for relaying metrics between group members.
type HTTPClient interface {
	HandleHTTP(ctx context.Context, p net.Peer) (http.Handler, error)
}

// MetricsHandlerForPeer returns a handler for retrieving metric information from a peer in this group
func (bp *BeaconProcess) MetricsHandlerForPeer(ctx context.Context, addr string) (http.Handler, error) {
	ctx, span := tracer.NewSpan(ctx, "bp.MetricsHandlerForPeer")
	defer span.End()

	if bp.group == nil {
		return nil, fmt.Errorf("%w for Beacon ID: %s", common.ErrNotPartOfGroup, bp.getBeaconID())
	}

	pc := bp.privGateway.ProtocolClient
	hc, ok := pc.(HTTPClient)
	if !ok {
		return nil, errors.New("the ProtocolClient implementation does not support metrics")
	}

	err := errors.New("empty node list, skipping metrics for now")

	//nolint:gocritic
	for _, n := range bp.group.Nodes {
		if n.Address() == addr {
			p := net.CreatePeer(n.Address(), n.TLS)
			h, e := hc.HandleHTTP(ctx, p)
			if e == nil {
				return h, nil
			}

			bp.log.Warnw("", "metrics", "Error while adding node", "address", n.Address(), "error", err)
			err = e
		}
	}

	return nil, err
}

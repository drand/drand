package core

import (
	"context"
	"errors"
	"net/http"
)

// MetricsHandlerForPeer returns a handler for retrieving metric information from a peer in this group
func (bp *BeaconProcess) MetricsHandlerForPeer(_ context.Context, _ string) (http.Handler, error) {
	//nolint:gocritic
	//ctx, span := metrics.NewSpan(ctx, "bp.MetricsHandlerForPeer")
	//defer span.End()
	//
	//if bp.group == nil {
	//	return nil, fmt.Errorf("%w for Beacon ID: %s", common.ErrNotPartOfGroup, bp.getBeaconID())
	//}

	return nil, errors.New("the ProtocolClient implementation does not support metrics")

	// TODO: fix the net.HTTPClient stuff for metrics
	//pc := bp.privGateway.ProtocolClient
	//hc, ok := pc.(net.HTTPClient)
	//if !ok {
	//	return nil, errors.New("the ProtocolClient implementation does not support metrics")
	//}
	//
	//err := errors.New("empty node list, skipping metrics for now")
	//
	////nolint:gocritic
	//for _, n := range bp.group.Nodes {
	//	if n.Address() == addr {
	//		p := net.CreatePeer(n.Address())
	//		h, e := hc.HandleHTTP(ctx, p)
	//		if e == nil {
	//			return h, nil
	//		}
	//
	//		bp.log.Warnw("", "metrics", "Error while adding node", "address", n.Address(), "error", err)
	//		err = e
	//	}
	//}
	//
	//return nil, err
}

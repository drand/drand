package net

import (
	"context"

	"google.golang.org/grpc/stats"

	"github.com/drand/drand/metrics"
)

type incomingConnectionsStatsHandler struct {
	stats.Handler
}

func (h incomingConnectionsStatsHandler) TagRPC(ctx context.Context, tagInfo *stats.RPCTagInfo) context.Context {
	// no-op
	return ctx
}

func (h incomingConnectionsStatsHandler) HandleRPC(ctx context.Context, rpcStats stats.RPCStats) {
	// no-op
}

func (h incomingConnectionsStatsHandler) TagConn(ctx context.Context, tagInfo *stats.ConnTagInfo) context.Context {
	metrics.IncomingConnectionTimestamp.WithLabelValues(tagInfo.RemoteAddr.String()).SetToCurrentTime()
	return ctx
}

func (h incomingConnectionsStatsHandler) HandleConn(ctx context.Context, connStats stats.ConnStats) {
	// no-op
}

var IncomingConnectionsStatsHandler = incomingConnectionsStatsHandler{}

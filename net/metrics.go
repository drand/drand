package net

import (
	"context"
	"strings"

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
	remoteAddr := tagInfo.RemoteAddr.String()
	parts := strings.Split(remoteAddr, ":")
	metrics.IncomingConnectionTimestamp.WithLabelValues(parts[0]).SetToCurrentTime()
	return ctx
}

func (h incomingConnectionsStatsHandler) HandleConn(ctx context.Context, connStats stats.ConnStats) {
	// no-op
}

var IncomingConnectionsStatsHandler = incomingConnectionsStatsHandler{}

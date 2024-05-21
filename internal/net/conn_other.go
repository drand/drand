//go:build conn_insecure

package net

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	insecure "google.golang.org/grpc/credentials/insecure"

	"github.com/drand/drand/v2/internal/metrics"
)

// conn retrieve an already existing conn to the given peer or create a new one.
// This version is the NON-TLS CONNECTION FOR TEST PURPOSES, it's behind a build tag that we use in our tests.
func (g *grpcClient) conn(p Peer) (*grpc.ClientConn, error) {
	// This is the NON-TLS version!
	// If you change anything here, don't forget to also change it in the TLS one in conn_tls.go

	g.Lock()
	defer g.Unlock()
	var err error

	// we try to retrieve an existing connection if available
	c, ok := g.conns[p.Address()]
	if ok && c.GetState() == connectivity.Shutdown {
		ok = false
		// we need to close the connection before deleting it to avoid goroutine leaks, done async
		go c.Close()
		delete(g.conns, p.Address())
		g.log.Warnw("non-TLS grpc conn in Shutdown state", "to", p.Address())
		metrics.OutgoingConnectionState.WithLabelValues(p.Address()).Set(float64(connectivity.Shutdown))
	}

	// otherwise we try to re-dial it
	if !ok {
		g.log.Debugw("initiating new non-TLS grpc conn", "to", p.Address())

		opts := append(
			[]grpc.DialOption{
				grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			},
			g.opts...,
		)

		c, err = grpc.NewClient(p.Address(), opts...)
		if err != nil {
			g.log.Errorw("error initiating a new non-TLS grpc conn", "to", p.Address(), "err", err)
			// We increase the GroupDialFailures counter when both failed
			metrics.GroupDialFailures.WithLabelValues(p.Address()).Inc()
		} else {
			g.log.Debugw("new non-TLS grpc conn established", "state", c.GetState(), "to", p.Address())
			g.conns[p.Address()] = c
			metrics.OutgoingConnections.Set(float64(len(g.conns)))
		}
	}

	// Emit the connection state regardless of whether it's a new or an existing connection
	if err == nil {
		metrics.OutgoingConnectionState.WithLabelValues(p.Address()).Set(float64(c.GetState()))
	}
	return c, err
}

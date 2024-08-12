package net

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	grpcmiddleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpcrecovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/internal/metrics"
	pdkg "github.com/drand/drand/v2/protobuf/dkg"
	"github.com/drand/drand/v2/protobuf/drand"
)

var isGrpcPrometheusMetricsRegisted = false
var state sync.Mutex

func registerGRPCMetrics(l log.Logger) error {
	if err := metrics.PrivateMetrics.Register(grpcprometheus.DefaultServerMetrics); err != nil {
		l.Warnw("", "grpc Listener", "failed metrics registration", "err", err)
		return err
	}

	isGrpcPrometheusMetricsRegisted = true
	return nil
}

// NewGRPCListenerForPrivate creates a new listener for the Public and Protocol APIs over GRPC. Note that this is
// using a regular, non-TLS listener, this is assuming the node is behind a reverse proxy doing TLS termination.
func NewGRPCListenerForPrivate(ctx context.Context, bindingAddr string, s Service, opts ...grpc.ServerOption) (Listener, error) {
	lis, err := net.Listen("tcp", bindingAddr)
	if err != nil {
		return nil, err
	}

	l := log.FromContextOrDefault(ctx)

	opts = append(opts,
		grpc.StreamInterceptor(
			grpcmiddleware.ChainStreamServer(
				grpcprometheus.StreamServerInterceptor,
				s.NodeVersionStreamValidator,
				grpcrecovery.StreamServerInterceptor(), // TODO (dlsniper): This turns panics into grpc errors. Do we want that?
			),
		),
		grpc.UnaryInterceptor(
			grpcmiddleware.ChainUnaryServer(
				grpcprometheus.UnaryServerInterceptor,
				s.NodeVersionValidator,
				grpcrecovery.UnaryServerInterceptor(), // TODO (dlsniper): This turns panics into grpc errors. Do we want that?
			),
		),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		// this limits the number of concurrent streams to each ServerTransport to prevent potential remote DoS
		//nolint:mnd
		grpc.MaxConcurrentStreams(256),
	)

	grpcServer := grpc.NewServer(opts...)

	// support GRPC health checking
	healthcheck := health.NewServer()
	healthgrpc.RegisterHealthServer(grpcServer, healthcheck)

	drand.RegisterPublicServer(grpcServer, s)
	drand.RegisterProtocolServer(grpcServer, s)
	pdkg.RegisterDKGControlServer(grpcServer, s)

	g := &grpcListener{
		Service:      s,
		grpcServer:   grpcServer,
		lis:          lis,
		healthServer: healthcheck,
	}

	grpcprometheus.Register(grpcServer)
	drand.RegisterMetricsServer(grpcServer, s)

	state.Lock()
	defer state.Unlock()
	if !isGrpcPrometheusMetricsRegisted {
		if err := registerGRPCMetrics(l); err != nil {
			return nil, err
		}
	}

	return g, nil
}

// NewRESTListenerForPublic creates a new listener for the Public API over REST.
func NewRESTListenerForPublic(ctx context.Context, bindingAddr string, handler http.Handler) (Listener, error) {
	lis, err := net.Listen("tcp", bindingAddr)
	if err != nil {
		return nil, err
	}

	l := log.FromContextOrDefault(ctx)

	g := &restListener{
		lis: lis,
		l:   l,
	}
	g.restServer = &http.Server{
		Addr:              bindingAddr,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           handler,
	}

	return g, nil
}

type restListener struct {
	restServer *http.Server
	lis        net.Listener
	l          log.Logger
}

func (g *restListener) Addr() string {
	return g.lis.Addr().String()
}

func (g *restListener) Start() {
	_ = g.restServer.Serve(g.lis)
}

func (g *restListener) Stop(ctx context.Context) {
	if err := g.lis.Close(); err != nil {
		g.l.Debugw("", "grpc listener", "grpc shutdown", "err", err)
	}
	if err := g.restServer.Shutdown(ctx); err != nil {
		g.l.Debugw("", "grpc listener", "http shutdown", "err", err)
	}
}

type grpcListener struct {
	Service
	grpcServer   *grpc.Server
	lis          net.Listener
	healthServer *health.Server
}

func (g *grpcListener) Addr() string {
	return g.lis.Addr().String()
}

func (g *grpcListener) Start() {
	go func() {
		g.healthServer.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
		_ = g.grpcServer.Serve(g.lis)
	}()
}

func (g *grpcListener) Stop(_ context.Context) {
	g.healthServer.SetServingStatus("", healthgrpc.HealthCheckResponse_NOT_SERVING)
	g.grpcServer.Stop()
	_ = g.lis.Close()
}

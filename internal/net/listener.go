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
	"github.com/weaveworks/common/httpgrpc"
	httpgrpcserver "github.com/weaveworks/common/httpgrpc/server"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	pdkg "github.com/drand/drand/protobuf/dkg"

	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/metrics"
	"github.com/drand/drand/protobuf/drand"
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

// NewGRPCListenerForPrivate creates a new listener for the Public and Protocol APIs over GRPC.
func NewGRPCListenerForPrivate(ctx context.Context, bindingAddr string, s Service, opts ...grpc.ServerOption) (Listener, error) {
	lis, err := net.Listen("tcp", bindingAddr)
	if err != nil {
		return nil, err
	}

	l := log.FromContextOrDefault(ctx)

	opts = append(opts,
		grpc.StreamInterceptor(
			grpcmiddleware.ChainStreamServer(
				otelgrpc.StreamServerInterceptor(),
				grpcprometheus.StreamServerInterceptor,
				s.NodeVersionStreamValidator,
				grpcrecovery.StreamServerInterceptor(), // TODO (dlsniper): This turns panics into grpc errors. Do we want that?
			),
		),
		grpc.UnaryInterceptor(
			grpcmiddleware.ChainUnaryServer(
				otelgrpc.UnaryServerInterceptor(),
				grpcprometheus.UnaryServerInterceptor,
				s.NodeVersionValidator,
				grpcrecovery.UnaryServerInterceptor(), // TODO (dlsniper): This turns panics into grpc errors. Do we want that?
			),
		),
	)

	grpcServer := grpc.NewServer(opts...)

	drand.RegisterPublicServer(grpcServer, s)
	drand.RegisterProtocolServer(grpcServer, s)
	pdkg.RegisterDKGControlServer(grpcServer, s)

	g := &grpcListener{
		Service:    s,
		grpcServer: grpcServer,
		lis:        lis,
	}

	//// TODO: remove httpgrpcserver from our codebase
	httpgrpc.RegisterHTTPServer(grpcServer, httpgrpcserver.NewServer(metrics.GroupHandler(l)))
	grpcprometheus.Register(grpcServer)

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
	grpcServer *grpc.Server
	lis        net.Listener
}

func (g *grpcListener) Addr() string {
	return g.lis.Addr().String()
}

func (g *grpcListener) Start() {
	go func() {
		_ = g.grpcServer.Serve(g.lis)
	}()
}

func (g *grpcListener) Stop(_ context.Context) {
	g.grpcServer.Stop()
	_ = g.lis.Close()
}

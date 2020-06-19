package net

import (
	"context"
	"net"
	"net/http"

	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/protobuf/drand"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	http_grpc "github.com/weaveworks/common/httpgrpc"
	http_grpc_server "github.com/weaveworks/common/httpgrpc/server"
	"google.golang.org/grpc"
)

// NewGRPCListenerForPrivate creates a new listener for the Public and Protocol APIs over GRPC with no TLS.
func NewGRPCListenerForPrivate(ctx context.Context, addr string, s Service, opts ...grpc.ServerOption) (Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	opts = append(opts, grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor))
	opts = append(opts, grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor))
	grpcServer := grpc.NewServer(opts...)
	g := &grpcListener{
		Service:    s,
		grpcServer: grpcServer,
		lis:        l,
	}
	drand.RegisterProtocolServer(g.grpcServer, g.Service)
	drand.RegisterPublicServer(g.grpcServer, g.Service)
	http_grpc.RegisterHTTPServer(g.grpcServer, http_grpc_server.NewServer(metrics.GroupHandler()))
	grpc_prometheus.Register(g.grpcServer)
	metrics.PrivateMetrics.Register(grpc_prometheus.DefaultServerMetrics)
	return g, nil
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
	go g.grpcServer.Serve(g.lis)
}

func (g *grpcListener) Stop(ctx context.Context) {
	g.lis.Close()
	g.grpcServer.Stop()
}

// NewRESTListenerForPublic creates a new listener for the Public API over HTTP/JSON without TLS.
func NewRESTListenerForPublic(ctx context.Context, addr string, handler http.Handler) (Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	restServer := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	g := &restListener{
		restServer: restServer,
		lis:        l,
	}
	return g, nil
}

type restListener struct {
	restServer *http.Server
	lis        net.Listener
}

func (g *restListener) Addr() string {
	return g.lis.Addr().String()
}

func (g *restListener) Start() {
	g.restServer.Serve(g.lis)
}

func (g *restListener) Stop(ctx context.Context) {
	if err := g.lis.Close(); err != nil {
		log.DefaultLogger().Debug("grpc insecure listener", "grpc shutdown", "err", err)
	}
	if err := g.restServer.Shutdown(ctx); err != nil {
		log.DefaultLogger().Debug("grpc insecure listener", "http shutdown", "err", err)
	}
}

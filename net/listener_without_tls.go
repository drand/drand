package net

import (
	"context"
	"net"
	"net/http"

	"github.com/drand/drand/protobuf/drand"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
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
	grpc_prometheus.Register(g.grpcServer)
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
	g.lis.Close()
	g.restServer.Shutdown(ctx)
}

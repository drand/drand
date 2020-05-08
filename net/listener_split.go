package net

import (
	"context"
	"net"
	"net/http"

	"github.com/drand/drand/protobuf/drand"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
)

// NewGRPCListenerForPublicAndProtocol creates a new listener for the Public and Protocol APIs over GRPC with no TLS.
func NewGRPCListenerForPublicAndProtocol(addr string, s Service, opts ...grpc.ServerOption) Listener {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		panic("tcp listener: " + err.Error())
	}
	mux := cmux.New(l)
	opts = append(opts, grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor))
	opts = append(opts, grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor))
	grpcServer := grpc.NewServer(opts...)
	g := &grpcListener{
		Service:    s,
		grpcServer: grpcServer,
		mux:        mux,
		lis:        l,
	}
	drand.RegisterProtocolServer(g.grpcServer, g.Service)
	drand.RegisterPublicServer(g.grpcServer, g.Service)
	grpc_prometheus.Register(g.grpcServer)
	return g
}

type grpcListener struct {
	Service
	grpcServer *grpc.Server
	mux        cmux.CMux
	lis        net.Listener
}

func (g *grpcListener) Start() {
	l := g.mux.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
	go g.grpcServer.Serve(l)
	g.mux.Serve()
}

func (g *grpcListener) Stop() {
	g.lis.Close()
	g.grpcServer.Stop()
}

// NewRESTListenerForPublic creates a new listener for the Public API over HTTP/JSON without TLS.
func NewRESTListenerForPublic(addr string, s Service, opts ...grpc.ServerOption) Listener {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		panic("tcp listener: " + err.Error())
	}
	mux := cmux.New(l)
	opts = append(opts, grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor))
	opts = append(opts, grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor))
	grpcServer := grpc.NewServer(opts...)
	// REST api
	o := runtime.WithMarshalerOption("*", defaultJSONMarshaller)
	gwMux := runtime.NewServeMux(o)
	proxyClient := &drandProxy{s}
	ctx := context.TODO()
	if err := drand.RegisterPublicHandlerClient(ctx, gwMux, proxyClient); err != nil {
		panic(err)
	}
	restRouter := http.NewServeMux()
	restRouter.Handle("/", gwMux)
	restServer := &http.Server{
		Addr:    addr,
		Handler: grpcHandlerFunc(grpcServer, restRouter),
	}

	g := &restListener{
		Service:    s,
		grpcServer: grpcServer,
		restServer: restServer,
		mux:        mux,
		lis:        l,
	}
	drand.RegisterPublicServer(g.grpcServer, g.Service)
	grpc_prometheus.Register(g.grpcServer)
	return g
}

type restListener struct {
	Service
	grpcServer *grpc.Server
	restServer *http.Server
	mux        cmux.CMux
	lis        net.Listener
}

func (g *restListener) Start() {
	l := g.mux.Match(cmux.Any())
	go g.restServer.Serve(l)
	g.mux.Serve()
}

func (g *restListener) Stop() {
	g.lis.Close()
	g.restServer.Shutdown(context.Background())
	g.grpcServer.Stop()
}

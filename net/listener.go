package net

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/protobuf/drand"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	http_grpc "github.com/weaveworks/common/httpgrpc"
	http_grpc_server "github.com/weaveworks/common/httpgrpc/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func registerGRPCMetrics() {
	if err := metrics.PrivateMetrics.Register(grpc_prometheus.DefaultServerMetrics); err != nil {
		log.DefaultLogger().Warn("grpc Listener", "failed metrics registration", "err", err)
	}
}

// NewGRPCListenerForPrivate creates a new listener for the Public and Protocol APIs over GRPC.
func NewGRPCListenerForPrivate(
	ctx context.Context,
	bindingAddr, certPath, keyPath string,
	s Service,
	insecure bool,
	opts ...grpc.ServerOption) (Listener, error) {
	lis, err := net.Listen("tcp", bindingAddr)
	if err != nil {
		return nil, err
	}

	if !insecure {
		grpcCreds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.Creds(grpcCreds))
	}
	opts = append(opts,
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor))
	grpcServer := grpc.NewServer(opts...)
	drand.RegisterPublicServer(grpcServer, s)
	drand.RegisterProtocolServer(grpcServer, s)

	var g Listener
	if insecure {
		g = &grpcListener{
			Service:    s,
			grpcServer: grpcServer,
			lis:        lis,
		}
	} else {
		x509KeyPair, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, err
		}

		gr := &restListener{
			restServer: buildTLSServer(grpcServer, &x509KeyPair),
		}
		gr.lis = tls.NewListener(lis, gr.restServer.TLSConfig)
		g = gr
	}
	http_grpc.RegisterHTTPServer(grpcServer, http_grpc_server.NewServer(metrics.GroupHandler()))
	grpc_prometheus.Register(grpcServer)
	registerGRPCMetrics()
	return g, nil
}

// NewRESTListenerForPublic creates a new listener for the Public API over REST with TLS.
func NewRESTListenerForPublic(
	ctx context.Context,
	bindingAddr, certPath, keyPath string,
	handler http.Handler,
	insecure bool) (Listener, error) {
	lis, err := net.Listen("tcp", bindingAddr)
	if err != nil {
		return nil, err
	}

	g := &restListener{
		lis: lis,
	}
	if insecure {
		g.restServer = &http.Server{
			Addr:    bindingAddr,
			Handler: handler,
		}
	} else {
		x509KeyPair, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, err
		}

		g.restServer = buildTLSServer(handler, &x509KeyPair)
		g.lis = tls.NewListener(lis, g.restServer.TLSConfig)
	}
	return g, nil
}

func buildTLSServer(httpHandler http.Handler, x509KeyPair *tls.Certificate) *http.Server {
	return &http.Server{
		Handler: httpHandler,
		TLSConfig: &tls.Config{
			// From https://blog.cloudflare.com/exposing-go-on-the-internet/

			// Causes servers to use Go's default ciphersuite preferences,
			// which are tuned to avoid attacks. Does nothing on clients.
			PreferServerCipherSuites: true,

			// Only use curves which have assembly implementations
			CurvePreferences: []tls.CurveID{
				tls.CurveP256,
				tls.X25519,
			},

			// Drand clients and servers are all modern software, and so we
			// can require TLS 1.2 and the best cipher suites.
			MinVersion: tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
			// End Cloudflare recommendations.

			Certificates: []tls.Certificate{*x509KeyPair},
			NextProtos:   []string{"h2"},
		},
	}
}

type restListener struct {
	restServer *http.Server
	lis        net.Listener
}

func (g *restListener) Addr() string {
	return g.lis.Addr().String()
}

func (g *restListener) Start() {
	_ = g.restServer.Serve(g.lis)
}

func (g *restListener) Stop(ctx context.Context) {
	if err := g.lis.Close(); err != nil {
		log.DefaultLogger().Debug("grpc listener", "grpc shutdown", "err", err)
	}
	if err := g.restServer.Shutdown(ctx); err != nil {
		log.DefaultLogger().Debug("grpc listener", "http shutdown", "err", err)
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

func (g *grpcListener) Stop(ctx context.Context) {
	_ = g.lis.Close()
	g.grpcServer.Stop()
}

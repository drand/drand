package net

import (
	"context"
	"crypto/tls"
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
	"google.golang.org/grpc/credentials"

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

	l := log.FromContextOrDefault(ctx)

	if !insecure {
		grpcCreds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.Creds(grpcCreds))
	}

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
	drand.RegisterDKGServer(grpcServer, s)

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
			l:          l,
		}
		gr.lis = tls.NewListener(lis, gr.restServer.TLSConfig)
		g = gr
	}

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

	l := log.FromContextOrDefault(ctx)

	g := &restListener{
		lis: lis,
		l:   l,
	}
	if insecure {
		g.restServer = &http.Server{
			Addr:              bindingAddr,
			ReadHeaderTimeout: 3 * time.Second,
			Handler:           handler,
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
		Handler:           httpHandler,
		ReadHeaderTimeout: 3 * time.Second,
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

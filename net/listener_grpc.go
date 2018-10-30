package net

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strings"

	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/nikkolasg/slog"
	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// grpcInsecureListener implements Listener using gRPC connections and regular HTTP
// connections for the JSON REST API.
// NOTE: This use cmux under the hood to be able to use non-tls connection. The
// reason of this relatively high costs (multiple routines etc) is described in
// the issue https://github.com/grpc/grpc-go/issues/555.
type grpcInsecureListener struct {
	Service
	grpcServer *grpc.Server
	restServer *http.Server
	mux        cmux.CMux
	lis        net.Listener
}

// NewTCPGrpcListener returns a gRPC listener using plain TCP connections
// without TLS. The listener will bind to the given address:port
// tuple.
func NewTCPGrpcListener(addr string, s Service, opts ...grpc.ServerOption) Listener {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		panic("tcp listener: " + err.Error())
	}

	mux := cmux.New(l)

	// grpc API
	grpcServer := grpc.NewServer(opts...)

	// REST api
	gwMux := runtime.NewServeMux(runtime.WithMarshalerOption("application/json", defaultJSONMarshaller))
	proxyClient := newProxyClient(s)
	ctx := context.TODO()
	if err := drand.RegisterRandomnessHandlerClient(ctx, gwMux, proxyClient); err != nil {
		panic(err)
	}
	if err = drand.RegisterInfoHandlerClient(context.Background(), gwMux, proxyClient); err != nil {
		panic(err)
	}
	restRouter := http.NewServeMux()
	newHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		gwMux.ServeHTTP(w, r)
	}

	restRouter.Handle("/", http.HandlerFunc(newHandler))
	restServer := &http.Server{
		Handler: restRouter,
	}

	g := &grpcInsecureListener{
		Service:    s,
		grpcServer: grpcServer,
		restServer: restServer,
		mux:        mux,
		lis:        l,
	}
	drand.RegisterRandomnessServer(g.grpcServer, g.Service)
	drand.RegisterBeaconServer(g.grpcServer, g.Service)
	drand.RegisterInfoServer(g.grpcServer, g.Service)
	dkg.RegisterDkgServer(g.grpcServer, g.Service)
	return g
}

func (g *grpcInsecureListener) Start() {
	grpcL := g.mux.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	restL := g.mux.Match(cmux.Any())

	go g.grpcServer.Serve(grpcL)
	go g.restServer.Serve(restL)
	g.mux.Serve()
}

func (g *grpcInsecureListener) Stop() {
	g.lis.Close()
	g.restServer.Shutdown(context.Background())
	g.grpcServer.Stop()
}

type grpcTLSListener struct {
	Service
	server     *http.Server
	grpcServer *grpc.Server
	// tls listener
	l net.Listener
}

func NewTLSGrpcListener(bindingAddr string, certPath, keyPath string, s Service, opts ...grpc.ServerOption) (Listener, error) {
	lis, err := net.Listen("tcp", bindingAddr)
	if err != nil {
		return nil, err
	}

	x509KeyPair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}

	grpcCreds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	serverOpts := append(opts, grpc.Creds(grpcCreds))
	grpcServer := grpc.NewServer(serverOpts...)
	drand.RegisterRandomnessServer(grpcServer, s)
	drand.RegisterInfoServer(grpcServer, s)
	drand.RegisterBeaconServer(grpcServer, s)
	dkg.RegisterDkgServer(grpcServer, s)

	gwMux := runtime.NewServeMux(runtime.WithMarshalerOption("application/json", defaultJSONMarshaller))
	proxy := &drandProxy{s, s}
	err = drand.RegisterRandomnessHandlerClient(context.Background(), gwMux, proxy)
	if err != nil {
		return nil, err
	}
	err = drand.RegisterInfoHandlerClient(context.Background(), gwMux, proxy)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/", gwMux)
	server := &http.Server{
		Handler: grpcHandlerFunc(grpcServer, mux),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{x509KeyPair},
			NextProtos:   []string{"h2"},
		},
	}

	tlsListener := tls.NewListener(lis, server.TLSConfig)
	g := &grpcTLSListener{
		Service:    s,
		server:     server,
		grpcServer: grpcServer,
		l:          tlsListener,
	}
	return g, nil
}

func (g *grpcTLSListener) Start() {
	if err := g.server.Serve(g.l); err != nil {
		slog.Debugf("grpc: tls listener start failed: %s", err)
	}
}

func (g *grpcTLSListener) Stop() {
	// Graceful stop not supported with HTTP Server
	// https://github.com/grpc/grpc-go/issues/1384
	if err := g.server.Shutdown(context.TODO()); err != nil {
		slog.Debugf("grpc: tls listener shutdown failed: %s", err)
	}
}

type drandProxy struct {
	r drand.RandomnessServer
	d drand.InfoServer
}

func (d *drandProxy) Public(c context.Context, r *drand.PublicRandRequest, opts ...grpc.CallOption) (*drand.PublicRandResponse, error) {
	return d.r.Public(c, r)
}
func (d *drandProxy) Private(c context.Context, r *drand.PrivateRandRequest, opts ...grpc.CallOption) (*drand.PrivateRandResponse, error) {
	return d.r.Private(c, r)
}

func (d *drandProxy) DistKey(c context.Context, r *drand.DistKeyRequest, opts ...grpc.CallOption) (*drand.DistKeyResponse, error) {
	return d.d.DistKey(c, r)
}

// grpcHandlerFunc returns an http.Handler that delegates to grpcServer on incoming gRPC
// connections or otherHandler otherwise. Copied from cockroachdb.
// taken from https://github.com/philips/grpc-gateway-example
func grpcHandlerFunc(grpcServer *grpc.Server, otherHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		// TODO(tamird): point to merged gRPC code rather than a PR.
		// This is a partial recreation of gRPC's internal checks https://github.com/grpc/grpc-go/pull/514/files#diff-95e9a25b738459a2d3030e1e6fa2a718R61
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			otherHandler.ServeHTTP(w, r)
		}
	})
}

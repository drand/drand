package net

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/drand/drand/metrics"
	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/grpc"
)

// drandProxy is used as a proxy between the REST API receiver and the gRPC
// endpoint. Normally, one would need to make another HTTP request to the
// grpc endpoint. Here we use a struct that directly calls the requested gRPC
// method since both the REST API and gRPC API lives on the same endpoint.
type drandProxy struct {
	r drand.PublicServer
}

var _ drand.PublicClient = (*drandProxy)(nil)

func (d *drandProxy) PublicRand(c context.Context, r *drand.PublicRandRequest, opts ...grpc.CallOption) (*drand.PublicRandResponse, error) {
	return d.r.PublicRand(c, r)
}
func (d *drandProxy) PublicRandStream(ctx context.Context, in *drand.PublicRandRequest, opts ...grpc.CallOption) (drand.Public_PublicRandStreamClient, error) {
	return nil, errors.New("streaming is not supported on HTTP endpoint")
}
func (d *drandProxy) PrivateRand(c context.Context, r *drand.PrivateRandRequest, opts ...grpc.CallOption) (*drand.PrivateRandResponse, error) {
	return d.r.PrivateRand(c, r)
}

func (d *drandProxy) DistKey(c context.Context, r *drand.DistKeyRequest, opts ...grpc.CallOption) (*drand.DistKeyResponse, error) {
	return d.r.DistKey(c, r)
}
func (d *drandProxy) Home(c context.Context, r *drand.HomeRequest, opts ...grpc.CallOption) (*drand.HomeResponse, error) {
	return d.r.Home(c, r)
}
func (d *drandProxy) Group(c context.Context, r *drand.GroupRequest, opts ...grpc.CallOption) (*drand.GroupPacket, error) {
	return d.r.Group(c, r)
}

// grpcHandlerFunc returns an http.Handler that delegates to grpcServer on
// incoming gRPC connections or otherHandler otherwise. Copied from cockroachdb.
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
			// record metrics for HTTP API endpoints
			switch r.URL.Path {
			case "/api":
				metrics.APICallCounter.WithLabelValues("home").Inc()
			case "/api/private":
				metrics.APICallCounter.WithLabelValues("private").Inc()
			case "/api/info/distkey":
				metrics.APICallCounter.WithLabelValues("distkey").Inc()
			case "/api/info/group":
				metrics.APICallCounter.WithLabelValues("group").Inc()
			default:
				// api/public can have additional path ServerParameters
				if strings.Contains(r.URL.Path, "/api/public") {
					metrics.APICallCounter.WithLabelValues("public").Inc()
				}
			}
			otherHandler.ServeHTTP(w, r)
		}
	})
}

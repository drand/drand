package core

import (
	"context"
	"errors"

	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/grpc"
)

// drandProxy is used as a proxy between a Public service (e.g. the node as a server)
// and a Public Client (the client consumed by the HTTP API)
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

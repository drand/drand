package core

import (
	"time"

	"github.com/dedis/drand/dkg"
	"google.golang.org/grpc"
)

type DrandOptions func(*drandOpts)

type drandOpts struct {
	grpcOpts   []grpc.DialOption
	dkgTimeout time.Duration
}

func newDrandOpts(opts ...DrandOptions) *drandOpts {
	d := &drandOpts{
		grpcOpts:   []grpc.DialOption{grpc.WithInsecure()},
		dkgTimeout: dkg.DefaultTimeout,
	}
	for i := range opts {
		opts[i](d)
	}
	return d
}

func WithGrpcOptions(opts ...grpc.DialOption) DrandOptions {
	return func(d *drandOpts) {
		d.grpcOpts = opts
	}
}

func WithDkgTimeout(t time.Duration) DrandOptions {
	return func(d *drandOpts) {
		d.dkgTimeout = t
	}
}

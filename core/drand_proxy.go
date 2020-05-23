package core

import (
	"context"
	"fmt"
	"net"

	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// drandProxy is used as a proxy between a Public service (e.g. the node as a server)
// and a Public Client (the client consumed by the HTTP API)
type drandProxy struct {
	r drand.PublicServer
}

// streamProxy directly relays mesages of the PublicRandResponse stream.
type streamProxy struct {
	ctx      context.Context
	cancel   context.CancelFunc
	incoming chan *drand.PublicRandResponse
	outgoing chan *drand.PublicRandResponse
}

func newStreamProxy(ctx context.Context) *streamProxy {
	ctx, cancel := context.WithCancel(ctx)
	s := streamProxy{
		ctx:      ctx,
		cancel:   cancel,
		incoming: make(chan *drand.PublicRandResponse, 0),
		outgoing: make(chan *drand.PublicRandResponse, 1),
	}
	go s.loop()
	return &s
}

func (s *streamProxy) Recv() (*drand.PublicRandResponse, error) {
	next, ok := <-s.outgoing
	if ok {
		return next, nil
	}
	return nil, fmt.Errorf("stream closed")
}

func (s *streamProxy) Send(next *drand.PublicRandResponse) error {
	select {
	case s.incoming <- next:
		return nil
	case <-s.ctx.Done():
		close(s.incoming)
		return s.ctx.Err()
	}
}

func (s *streamProxy) loop() {
	defer close(s.outgoing)
	for {
		select {
		case next, ok := <-s.incoming:
			if !ok {
				return
			}
			select {
			case s.outgoing <- next:
			case <-s.ctx.Done():
				return
			default:
			}
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *streamProxy) Close(err error) {
	s.cancel()
	close(s.incoming)
}

/* implement the grpc stream interface. not used since messages passed directly. */
func (s *streamProxy) SetHeader(metadata.MD) error {
	return nil
}
func (s *streamProxy) SendHeader(metadata.MD) error {
	return nil
}
func (s *streamProxy) SetTrailer(metadata.MD) {}

func (s *streamProxy) Context() context.Context {
	return peer.NewContext(s.ctx, &peer.Peer{Addr: &net.UnixAddr{}})
}
func (s *streamProxy) SendMsg(m interface{}) error {
	return nil
}
func (s *streamProxy) RecvMsg(m interface{}) error {
	return nil
}

func (s *streamProxy) Header() (metadata.MD, error) {
	return nil, nil
}

func (s *streamProxy) Trailer() metadata.MD {
	return nil
}
func (s *streamProxy) CloseSend() error {
	return nil
}

var _ drand.PublicClient = (*drandProxy)(nil)

func (d *drandProxy) PublicRand(c context.Context, r *drand.PublicRandRequest, opts ...grpc.CallOption) (*drand.PublicRandResponse, error) {
	return d.r.PublicRand(c, r)
}
func (d *drandProxy) PublicRandStream(ctx context.Context, in *drand.PublicRandRequest, opts ...grpc.CallOption) (drand.Public_PublicRandStreamClient, error) {
	srvr := newStreamProxy(ctx)

	go func() {
		err := d.r.PublicRandStream(in, srvr)
		srvr.Close(err)
	}()

	return srvr, nil
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

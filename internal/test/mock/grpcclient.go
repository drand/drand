package mock

import (
	"context"
	"github.com/drand/drand/common"
	"github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/client"
	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"net"
	"time"
)

type GrpcClient struct {
	s *Server
}

func NewGrpcClient(s *Server) *GrpcClient {
	return &GrpcClient{s: s}
}

func (c *GrpcClient) Get(ctx context.Context, round uint64) (client.Result, error) {
	return c.s.PublicRand(ctx, &drand.PublicRandRequest{
		Round:    round,
		Metadata: nil,
	})
}

func (c *GrpcClient) Watch(ctx context.Context) <-chan client.Result {
	proxy := newStreamProxy(ctx)
	go func() {
		err := c.s.PublicRandStream(&drand.PublicRandRequest{}, proxy)
		if err != nil {
			proxy.Close()
		}
	}()
	return proxy.outgoing
}

func (c *GrpcClient) Info(ctx context.Context) (*chain.Info, error) {
	resp, err := c.s.ChainInfo(ctx, nil)
	if err != nil {
		return nil, err
	}

	//return &chain.Info{
	//	PublicKey:   resp.PublicKey,
	//	Period:      time.Duration(resp.Period),
	//	Scheme:      resp.SchemeID,
	//	GenesisTime: resp.GenesisTime,
	//}, err
	return chain.InfoFromProto(resp)
}

func (c *GrpcClient) RoundAt(time time.Time) uint64 {
	return 0
}

func (c *GrpcClient) Close() error {
	return nil
}

// streamProxy directly relays messages of the PublicRandResponse stream.
type streamProxy struct {
	ctx      context.Context
	cancel   context.CancelFunc
	outgoing chan client.Result
}

func newStreamProxy(ctx context.Context) *streamProxy {
	ctx, cancel := context.WithCancel(ctx)
	s := streamProxy{
		ctx:      ctx,
		cancel:   cancel,
		outgoing: make(chan client.Result, 1),
	}
	return &s
}

func (s *streamProxy) Send(next *drand.PublicRandResponse) error {
	d := common.Beacon{
		Round:       next.Round,
		Signature:   next.Signature,
		PreviousSig: next.PreviousSignature,
	}
	select {
	case s.outgoing <- &d:
		return nil
	case <-s.ctx.Done():
		close(s.outgoing)
		return s.ctx.Err()
	default:
		return nil
	}
}

func (s *streamProxy) Close() {
	s.cancel()
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
func (s *streamProxy) SendMsg(_ interface{}) error {
	return nil
}
func (s *streamProxy) RecvMsg(_ interface{}) error {
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

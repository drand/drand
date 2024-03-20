package net

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding/gzip"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/common/tracer"
	"github.com/drand/drand/v2/protobuf/drand"
)

// ensure we implement all required interfaces
var _ Client = (*grpcClient)(nil)

// grpcClient implements Protocol, DKG, Metric and Public client functionalities
// using gRPC as its underlying mechanism
type grpcClient struct {
	sync.RWMutex
	conns         map[string]*grpc.ClientConn
	opts          []grpc.DialOption
	timeout       time.Duration
	healthTimeout time.Duration
	log           log.Logger
}

var defaultConnTimeout = 1 * time.Minute
var defaultHealthTimeout = 2 * time.Second

// NewGrpcClient returns an implementation of an InternalClient  and
// ExternalClient using gRPC connections
func NewGrpcClient(l log.Logger, opts ...grpc.DialOption) Client {
	client := grpcClient{
		opts:          opts,
		conns:         make(map[string]*grpc.ClientConn),
		timeout:       defaultConnTimeout,
		healthTimeout: defaultHealthTimeout,
		log:           l,
	}
	client.loadEnvironment()
	return &client
}

func (g *grpcClient) loadEnvironment() {
	opt := grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
		return proxy.Dial(ctx, "tcp", addr)
	})
	g.opts = append([]grpc.DialOption{opt}, g.opts...)
}

func (g *grpcClient) getTimeoutContext(ctx context.Context) (context.Context, context.CancelFunc) {
	g.RLock()
	defer g.RUnlock()
	return context.WithTimeout(ctx, g.timeout)
}

func (g *grpcClient) GetIdentity(ctx context.Context, p Peer,
	in *drand.IdentityRequest, _ ...CallOption) (*drand.IdentityResponse, error) {
	var resp *drand.IdentityResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewProtocolClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.GetIdentity(ctx, in)
	return resp, err
}

func (g *grpcClient) PublicRand(ctx context.Context, p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewPublicClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	return client.PublicRand(ctx, in)
}

const grpcClientRandStreamBacklog = 10

// PublicRandStream allows clients to stream randomness
// TODO: move that to core/ client
func (g *grpcClient) PublicRandStream(
	ctx context.Context,
	p Peer,
	in *drand.PublicRandRequest,
	_ ...CallOption) (chan *drand.PublicRandResponse, error) {
	var outCh = make(chan *drand.PublicRandResponse, grpcClientRandStreamBacklog)
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewPublicClient(c)
	stream, err := client.PublicRandStream(ctx, in)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			resp, err := stream.Recv()
			// EOF means the stream was closed "properly"
			if errors.Is(err, io.EOF) {
				close(outCh)
				return
			}
			if err != nil {
				// TODO should probably do stg different here but since we are
				// continuously stream, if stream stops, it means stg went
				// wrong
				close(outCh)
				return
			}
			select {
			case outCh <- resp:
			case <-ctx.Done():
				close(outCh)
				return
			}
		}
	}()
	return outCh, nil
}

func (g *grpcClient) ChainInfo(ctx context.Context, p Peer, in *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	var resp *drand.ChainInfoPacket
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewPublicClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.ChainInfo(ctx, in)
	return resp, err
}

func (g *grpcClient) PartialBeacon(ctx context.Context, p Peer, in *drand.PartialBeaconPacket, opts ...CallOption) error {
	ctx, span := tracer.NewSpan(ctx, "client.PartialBeacon")
	defer span.End()

	c, err := g.conn(ctx, p)
	if err != nil {
		return err
	}
	client := drand.NewProtocolClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	_, err = client.PartialBeacon(ctx, in, opts...)
	return err
}

// MaxSyncBuffer is the maximum number of queued rounds when syncing
const MaxSyncBuffer = 500

func (g *grpcClient) SyncChain(ctx context.Context, p Peer, in *drand.SyncRequest, _ ...CallOption) (chan *drand.BeaconPacket, error) {
	resp := make(chan *drand.BeaconPacket, MaxSyncBuffer)
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewProtocolClient(c)
	stream, err := client.SyncChain(ctx, in)
	if err != nil {
		return nil, err
	}
	go func() {
		defer close(resp)
		for {
			reply, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				g.log.Infow("", "grpc client", "chain sync", "error", "eof", "to", p.Address())
				g.log.Debugw(" --- STREAM EOF")
				return
			}
			if err != nil {
				g.log.Infow("", "grpc client", "chain sync", "error", err, "to", p.Address())
				g.log.Debugw(fmt.Sprintf("--- STREAM ERR: %s", err))
				return
			}
			select {
			case <-ctx.Done():
				g.log.Infow("", "grpc client", "chain sync", "error", "context done", "to", p.Address())
				g.log.Debugw(" --- STREAM CONTEXT DONE")
				return
			default:
				resp <- reply
			}
		}
	}()
	return resp, nil
}

func (g *grpcClient) Status(ctx context.Context, p Peer, in *drand.StatusRequest, opts ...grpc.CallOption) (*drand.StatusResponse, error) {
	var resp *drand.StatusResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewProtocolClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Status(ctx, in, opts...)
	return resp, err
}

func (g *grpcClient) Stop() {
	g.Lock()
	defer g.Unlock()
	for _, c := range g.conns {
		_ = c.Close()
	}
	g.conns = make(map[string]*grpc.ClientConn)
}

func (g *grpcClient) GetMetrics(ctx context.Context, addr string) (string, error) {
	g.log.Debugw("GetMetrics grpcClient called", "target_addr", addr)
	p := CreatePeer(addr)
	var resp *drand.MetricsResponse
	// remote metrics are not group specific for now.
	in := &drand.MetricsRequest{}
	c, err := g.conn(ctx, p)
	if err != nil {
		return "", err
	}
	client := drand.NewMetricsClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	// we try to use compression since it's requesting a large string
	resp, err = client.Metrics(ctx, in, grpc.UseCompressor(gzip.Name))
	return string(resp.GetMetrics()), err
}

// ListBeaconIDs returns a list of all beacon ids
func (g *grpcClient) ListBeaconIDs(ctx context.Context, p Peer) (*drand.ListBeaconIDsResponse, error) {
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}

	client := drand.NewPublicClient(c)
	return client.ListBeaconIDs(context.Background(), &drand.ListBeaconIDsRequest{})
}

func (g *grpcClient) Check(ctx context.Context, p Peer) error {
	c, err := g.conn(ctx, p)
	if err != nil {
		return err
	}

	client := healthgrpc.NewHealthClient(c)
	//nolint:gomnd
	tctx, cancel := context.WithTimeout(ctx, g.healthTimeout)
	defer cancel()
	resp, err := client.Check(tctx, &healthgrpc.HealthCheckRequest{})
	if err != nil {
		return err
	}

	if resp.GetStatus() != healthgrpc.HealthCheckResponse_SERVING {
		return fmt.Errorf("health: %q not serving", p.Address())
	}

	return nil
}

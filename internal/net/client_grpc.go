package net

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/drand/drand/common/log"
	"github.com/drand/drand/common/tracer"
	"github.com/drand/drand/internal/metrics"
	"github.com/drand/drand/protobuf/drand"
	"github.com/weaveworks/common/httpgrpc"
	httpgrpcserver "github.com/weaveworks/common/httpgrpc/server"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

var _ Client = (*grpcClient)(nil)

// grpcClient implements both Protocol and Control functionalities
// using gRPC as its underlying mechanism
type grpcClient struct {
	sync.Mutex
	conns    map[string]*grpc.ClientConn
	opts     []grpc.DialOption
	timeout  time.Duration
	log      log.Logger
	insecure bool
}

var defaultTimeout = 1 * time.Minute

// NewGrpcClient returns an implementation of an InternalClient  and
// ExternalClient using gRPC connections
func NewGrpcClient(l log.Logger, opts ...grpc.DialOption) Client {
	client := grpcClient{
		opts:     opts,
		conns:    make(map[string]*grpc.ClientConn),
		timeout:  defaultTimeout,
		log:      l,
		insecure: false,
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
	g.Lock()
	defer g.Unlock()
	clientDeadline := time.Now().Add(g.timeout)
	return context.WithDeadline(ctx, clientDeadline)
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

func (g *grpcClient) Home(ctx context.Context, p Peer, in *drand.HomeRequest) (*drand.HomeResponse, error) {
	var resp *drand.HomeResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewPublicClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Home(ctx, in)
	return resp, err
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

// conn retrieve an already existing conn to the given peer or create a new one
func (g *grpcClient) conn(ctx context.Context, p Peer) (*grpc.ClientConn, error) {
	g.Lock()
	defer g.Unlock()
	var err error

	c, ok := g.conns[p.Address()]
	if ok && c.GetState() == connectivity.Shutdown {
		ok = false
		delete(g.conns, p.Address())
		metrics.OutgoingConnectionState.WithLabelValues(p.Address()).Set(float64(c.GetState()))
	}

	if !ok {
		g.log.Debugw("", "grpc client", "initiating", "to", p.Address())
		var opts []grpc.DialOption

		if !p.IsTLS() {
			c, err = grpc.Dial(p.Address(), append(g.opts, grpc.WithTransportCredentials(insecure.NewCredentials()))...)
			if err != nil {
				metrics.GroupDialFailures.WithLabelValues(p.Address()).Inc()
			}
			g.conns[p.Address()] = c
		} else {
			config := &tls.Config{MinVersion: tls.VersionTLS12}
			opts = append(opts, g.opts...)
			opts = append(opts,
				grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
				grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()),
				grpc.WithTransportCredentials(credentials.NewTLS(config)),
			)

			c, err = grpc.DialContext(ctx, p.Address(), opts...)
			if err != nil {
				metrics.GroupDialFailures.WithLabelValues(p.Address()).Inc()
				g.log.Errorw("error initiating a new grpc conn", "to", p.Address(), "err", err)
			} else {
				g.conns[p.Address()] = c
			}
		}
	}

	// Emit the connection state regardless of whether it's a new or an existing connection
	if err == nil {
		metrics.OutgoingConnectionState.WithLabelValues(p.Address()).Set(float64(c.GetState()))
	}

	metrics.OutgoingConnections.Set(float64(len(g.conns)))
	return c, err
}

func (g *grpcClient) Stop() {
	g.Lock()
	defer g.Unlock()
	for _, c := range g.conns {
		_ = c.Close()
	}
	g.conns = make(map[string]*grpc.ClientConn)
}

type httpHandler struct {
	httpgrpc.HTTPClient
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := httpgrpcserver.HTTPRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := h.Handle(r.Context(), req)
	if err != nil {
		var ok bool
		resp, ok = httpgrpc.HTTPResponseFromError(err)

		if !ok {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := httpgrpcserver.WriteResponse(w, resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (g *grpcClient) HandleHTTP(ctx context.Context, p Peer) (http.Handler, error) {
	conn, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := httpgrpc.NewHTTPClient(conn)

	return &httpHandler{client}, nil
}

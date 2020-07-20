package net

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/protobuf/drand"
	"github.com/weaveworks/common/httpgrpc"
	httpgrpcserver "github.com/weaveworks/common/httpgrpc/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var _ Client = (*grpcClient)(nil)

// grpcClient implements both Protocol and Control functionalities
// using gRPC as its underlying mechanism
type grpcClient struct {
	sync.Mutex
	conns   map[string]*grpc.ClientConn
	opts    []grpc.DialOption
	timeout time.Duration
	manager *CertManager
}

var defaultTimeout = 1 * time.Minute

// NewGrpcClient returns an implementation of an InternalClient  and
// ExternalClient using gRPC connections
func NewGrpcClient(opts ...grpc.DialOption) Client {
	return &grpcClient{
		opts:    opts,
		conns:   make(map[string]*grpc.ClientConn),
		timeout: defaultTimeout,
	}
}

// NewGrpcClientFromCertManager returns a Client using gRPC with the given trust
// store of certificates.
func NewGrpcClientFromCertManager(c *CertManager, opts ...grpc.DialOption) Client {
	client := NewGrpcClient(opts...).(*grpcClient)
	client.manager = c
	return client
}

// NewGrpcClientWithTimeout returns a Client using gRPC using fixed timeout for
// method calls.
func NewGrpcClientWithTimeout(timeout time.Duration, opts ...grpc.DialOption) Client {
	c := NewGrpcClient(opts...).(*grpcClient)
	c.timeout = timeout
	return c
}

func (g *grpcClient) getTimeoutContext(ctx context.Context) (context.Context, context.CancelFunc) {
	g.Lock()
	defer g.Unlock()
	clientDeadline := time.Now().Add(g.timeout)
	return context.WithDeadline(ctx, clientDeadline)
}

func (g *grpcClient) GetIdentity(ctx context.Context, p Peer, in *drand.IdentityRequest, opts ...CallOption) (*drand.Identity, error) {
	var resp *drand.Identity
	c, err := g.conn(p)
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
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewPublicClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	return client.PublicRand(ctx, in)
}

const grpcClientRandStreamBacklog = 10

// XXX move that to core/ client
func (g *grpcClient) PublicRandStream(
	ctx context.Context,
	p Peer,
	in *drand.PublicRandRequest,
	opts ...CallOption) (chan *drand.PublicRandResponse, error) {
	var outCh = make(chan *drand.PublicRandResponse, grpcClientRandStreamBacklog)
	c, err := g.conn(p)
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
			if err == io.EOF {
				close(outCh)
				return
			}
			if err != nil {
				// XXX should probably do stg different here but since we are
				// continuously stream, if stream stops, it means stg went
				// wrong; it should never EOF
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

func (g *grpcClient) PrivateRand(ctx context.Context, p Peer, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	var resp *drand.PrivateRandResponse
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewPublicClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()

	resp, err = client.PrivateRand(ctx, in)
	return resp, err
}

func (g *grpcClient) ChainInfo(ctx context.Context, p Peer, in *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	var resp *drand.ChainInfoPacket
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewPublicClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.ChainInfo(ctx, in)
	return resp, err
}

func (g *grpcClient) PushDKGInfo(ctx context.Context, p Peer, in *drand.DKGInfoPacket, opts ...grpc.CallOption) error {
	c, err := g.conn(p)
	if err != nil {
		return err
	}
	client := drand.NewProtocolClient(c)
	_, err = client.PushDKGInfo(ctx, in, opts...)
	return err
}

func (g *grpcClient) SignalDKGParticipant(ctx context.Context, p Peer, in *drand.SignalDKGPacket, opts ...CallOption) error {
	c, err := g.conn(p)
	if err != nil {
		return err
	}
	client := drand.NewProtocolClient(c)
	_, err = client.SignalDKGParticipant(ctx, in, opts...)
	return err
}

func (g *grpcClient) BroadcastDKG(ctx context.Context, p Peer, in *drand.DKGPacket, opts ...CallOption) error {
	c, err := g.conn(p)
	if err != nil {
		return err
	}
	client := drand.NewProtocolClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	_, err = client.BroadcastDKG(ctx, in, opts...)
	return err
}

func (g *grpcClient) PartialBeacon(ctx context.Context, p Peer, in *drand.PartialBeaconPacket, opts ...CallOption) error {
	c, err := g.conn(p)
	if err != nil {
		return err
	}
	client := drand.NewProtocolClient(c)
	ctx, _ = g.getTimeoutContext(ctx)
	_, err = client.PartialBeacon(ctx, in, opts...)
	return err
}

// MaxSyncBuffer is the maximum number of queued rounds when syncing
const MaxSyncBuffer = 100

func (g *grpcClient) SyncChain(ctx context.Context, p Peer, in *drand.SyncRequest, opts ...CallOption) (chan *drand.BeaconPacket, error) {
	resp := make(chan *drand.BeaconPacket, MaxSyncBuffer)
	c, err := g.conn(p)
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
			if err == io.EOF {
				log.DefaultLogger().Info("grpc client", "chain sync", "error", "eof", "to", p.Address())
				fmt.Println(" --- STREAM EOF")
				return
			}
			if err != nil {
				log.DefaultLogger().Info("grpc client", "chain sync", "error", err, "to", p.Address())
				fmt.Println(" --- STREAM ERR:", err)
				return
			}
			select {
			case <-ctx.Done():
				log.DefaultLogger().Info("grpc client", "chain sync", "error", "context done", "to", p.Address())
				fmt.Println(" --- STREAM CONTEXT DONE")
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
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewPublicClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Home(ctx, in)
	return resp, err
}

// conn retrieve an already existing conn to the given peer or create a new one
func (g *grpcClient) conn(p Peer) (*grpc.ClientConn, error) {
	g.Lock()
	defer g.Unlock()
	var err error
	c, ok := g.conns[p.Address()]
	if !ok {
		log.DefaultLogger().Debug("grpc client", "initiating", "to", p.Address(), "tls", p.IsTLS())
		if !p.IsTLS() {
			c, err = grpc.Dial(p.Address(), append(g.opts, grpc.WithInsecure())...)
			if err != nil {
				metrics.GroupDialFailures.WithLabelValues(p.Address()).Inc()
			}
		} else {
			var opts []grpc.DialOption
			opts = append(opts, g.opts...)
			if g.manager != nil {
				pool := g.manager.Pool()
				creds := credentials.NewClientTLSFromCert(pool, "")
				opts = append(opts, grpc.WithTransportCredentials(creds))
			} else {
				config := &tls.Config{}
				opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(config)))
			}
			c, err = grpc.Dial(p.Address(), opts...)
			if err != nil {
				metrics.GroupDialFailures.WithLabelValues(p.Address()).Inc()
			}
		}
		g.conns[p.Address()] = c
		metrics.GroupConnections.Set(float64(len(g.conns)))
	}
	return c, err
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

func (g *grpcClient) HandleHTTP(p Peer) (http.Handler, error) {
	conn, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := httpgrpc.NewHTTPClient(conn)

	return &httpHandler{client}, nil
}

package net

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/protobuf/drand"
	"github.com/nikkolasg/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var _ Client = (*grpcClient)(nil)

//var defaultJSONMarshaller = &runtime.JSONBuiltin{}
var defaultJSONMarshaller = &HexJSON{}

// grpcClient implements both Protocol and Control functionalities
// using gRPC as its underlying mechanism
type grpcClient struct {
	sync.Mutex
	conns    map[string]*grpc.ClientConn
	opts     []grpc.DialOption
	timeout  time.Duration
	manager  *CertManager
	failFast grpc.CallOption
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

func (g *grpcClient) SetTimeout(p time.Duration) {
	g.Lock()
	defer g.Unlock()
	g.timeout = p
}

func (g *grpcClient) PublicRand(ctx context.Context, p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	var resp *drand.PublicRandResponse
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewPublicClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.PublicRand(ctx, in)
	return resp, err
}

func (g *grpcClient) PublicRandStream(ctx context.Context, p Peer, in *drand.PublicRandRequest, opts ...CallOption) (chan *drand.PublicRandResponse, error) {
	var outCh = make(chan *drand.PublicRandResponse, 10)
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewPublicClient(c)
	ctx, _ = g.getTimeoutContext(ctx)
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

func (g *grpcClient) Group(ctx context.Context, p Peer, in *drand.GroupRequest) (*drand.GroupResponse, error) {
	var resp *drand.GroupResponse
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewPublicClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Group(ctx, in)
	return resp, err
}
func (g *grpcClient) DistKey(ctx context.Context, p Peer, in *drand.DistKeyRequest) (*drand.DistKeyResponse, error) {
	var resp *drand.DistKeyResponse
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewPublicClient(c)
	resp, err = client.DistKey(ctx, in)
	return resp, err
}
func (g *grpcClient) PrepareDKGGroup(ctx context.Context, p Peer, in *drand.PrepareDKGPacket, opts ...CallOption) (*drand.GroupPacket, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewProtocolClient(c)
	//ctx, cancel := g.getTimeoutContext(ctx)
	//defer cancel()
	return client.PrepareDKGGroup(ctx, in, opts...)

}

func (g *grpcClient) FreshDKG(ctx context.Context, p Peer, in *drand.DKGPacket, opts ...CallOption) (*drand.Empty, error) {
	var resp *drand.Empty
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewProtocolClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()

	resp, err = client.FreshDKG(ctx, in, opts...)
	return resp, err
}

func (g *grpcClient) ReshareDKG(ctx context.Context, p Peer, in *drand.ResharePacket, opts ...CallOption) (*drand.Empty, error) {
	var resp *drand.Empty
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewProtocolClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()

	resp, err = client.ReshareDKG(ctx, in, opts...)
	return resp, err
}

func (g *grpcClient) NewBeacon(p Peer, in *drand.BeaconPacket, opts ...CallOption) (*drand.Empty, error) {
	do := func() (*drand.Empty, error) {
		c, err := g.conn(p)
		if err != nil {
			return nil, err
		}
		client := drand.NewProtocolClient(c)
		ctx, _ := g.getTimeoutContext(context.Background())
		return client.NewBeacon(ctx, in, opts...)
	}
	if resp, err := do(); err != nil && strings.Contains(err.Error(), "connection error") {
		g.deleteConn(p)
		return do()
		//return resp, err
	} else {
		return resp, err
	}
}

const SyncBlockKey = "sync"

func (g *grpcClient) SyncChain(ctx context.Context, p Peer, in *drand.SyncRequest, opts ...CallOption) (chan *drand.SyncResponse, error) {
	resp := make(chan *drand.SyncResponse)
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
				fmt.Println(" --- STREAM EOF")
				break
			}
			if err != nil {
				fmt.Println(" --- STREAM ERR:", err)
				break
			}
			select {
			case <-ctx.Done():
				fmt.Println(" --- STREAM CONTEXT DONE")
				break
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

func (g *grpcClient) deleteConn(p Peer) {
	g.Lock()
	defer g.Unlock()
	delete(g.conns, p.Address())
}

// conn retrieve an already existing conn to the given peer or create a new one
func (g *grpcClient) conn(p Peer) (*grpc.ClientConn, error) {
	g.Lock()
	defer g.Unlock()
	var err error
	c, ok := g.conns[p.Address()]
	if !ok {
		slog.Debugf("grpc-client: attempting connection to %s (TLS %v)", p.Address(), p.IsTLS())
		if !p.IsTLS() {
			c, err = grpc.Dial(p.Address(), append(g.opts, grpc.WithInsecure())...)
		} else {
			var opts []grpc.DialOption
			for _, o := range g.opts {
				opts = append(opts, o)
			}
			if g.manager != nil {
				pool := g.manager.Pool()
				creds := credentials.NewClientTLSFromCert(pool, "")
				opts = append(opts, grpc.WithTransportCredentials(creds))
			} else {
				config := &tls.Config{}
				opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(config)))
			}
			c, err = grpc.Dial(p.Address(), opts...)
		}
		g.conns[p.Address()] = c
	}
	return c, err
}

// proxyClient is used by the gRPC json gateway to dispatch calls to the
// underlying gRPC server. It needs only to implement the public facing API
type proxyClient struct {
	s Service
}

func newProxyClient(s Service) *proxyClient {
	return &proxyClient{s}
}

func (p *proxyClient) Public(c context.Context, in *drand.PublicRandRequest, opts ...grpc.CallOption) (*drand.PublicRandResponse, error) {
	return p.s.PublicRand(c, in)
}
func (p *proxyClient) Private(c context.Context, in *drand.PrivateRandRequest, opts ...grpc.CallOption) (*drand.PrivateRandResponse, error) {
	return p.s.PrivateRand(c, in)
}
func (p *proxyClient) DistKey(c context.Context, in *drand.DistKeyRequest, opts ...grpc.CallOption) (*drand.DistKeyResponse, error) {
	return p.s.DistKey(c, in)
}

func (p *proxyClient) Home(c context.Context, in *drand.HomeRequest, opts ...grpc.CallOption) (*drand.HomeResponse, error) {
	return p.s.Home(c, in)
}

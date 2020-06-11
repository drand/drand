package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/protobuf/drand"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type grpcClient struct {
	address string
	client  drand.PublicClient
}

// New creates a drand client backed by a GRPC connection.
func New(address string, certPath string, insecure bool) (client.Client, error) {
	opts := []grpc.DialOption{}
	if certPath != "" {
		creds, err := credentials.NewClientTLSFromFile(certPath, "")
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else if insecure {
		opts = append(opts, grpc.WithInsecure())
	} else {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	}
	opts = append(opts,
		grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor),
		grpc.WithStreamInterceptor(grpc_prometheus.StreamClientInterceptor),
	)
	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, err
	}
	return &grpcClient{address, drand.NewPublicClient(conn)}, nil
}

func asRD(r *drand.PublicRandResponse) *client.RandomData {
	return &client.RandomData{
		Rnd:               r.Round,
		Random:            r.Randomness,
		Sig:               r.Signature,
		PreviousSignature: r.PreviousSignature,
	}
}

// String returns the name of this client.
func (g *grpcClient) String() string {
	return fmt.Sprintf("GRPC(%q)", g.address)
}

// Get returns a the randomness at `round` or an error.
func (g *grpcClient) Get(ctx context.Context, round uint64) (client.Result, error) {
	curr, err := g.client.PublicRand(ctx, &drand.PublicRandRequest{Round: round})
	if err != nil {
		return nil, err
	}
	if curr == nil {
		return nil, errors.New("No received randomness. Unexpected gPRC response")
	}

	return asRD(curr), nil
}

// Watch returns new randomness as it becomes available.
func (g *grpcClient) Watch(ctx context.Context) <-chan client.Result {
	stream, err := g.client.PublicRandStream(ctx, &drand.PublicRandRequest{Round: 0})
	ch := make(chan client.Result, 1)
	if err != nil {
		close(ch)
		return ch
	}
	go translate(stream, ch)
	return ch
}

// Info returns information about the chain.
func (g *grpcClient) Info(ctx context.Context) (*chain.Info, error) {
	proto, err := g.client.ChainInfo(ctx, &drand.ChainInfoRequest{})
	if err != nil {
		return nil, err
	}
	if proto == nil {
		return nil, errors.New("No received group. Unexpected gPRC response")
	}
	return chain.InfoFromProto(proto)
}

func translate(stream drand.Public_PublicRandStreamClient, out chan<- client.Result) {
	defer close(out)
	for {
		next, err := stream.Recv()
		if err != nil || stream.Context().Err() != nil {
			return
		}
		out <- asRD(next)
	}
}

func (g *grpcClient) RoundAt(t time.Time) uint64 {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := g.client.ChainInfo(ctx, &drand.ChainInfoRequest{})
	if err != nil {
		return 0
	}
	return chain.CurrentRound(t.Unix(), time.Second*time.Duration(info.Period), info.GenesisTime)
}

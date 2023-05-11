package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	grpcProm "github.com/grpc-ecosystem/go-grpc-prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	grpcInsec "google.golang.org/grpc/credentials/insecure"

	"github.com/drand/drand/client"
	chain2 "github.com/drand/drand/common/chain"
	client2 "github.com/drand/drand/common/client"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
)

const grpcDefaultTimeout = 5 * time.Second

type grpcClient struct {
	address   string
	chainHash []byte
	client    drand.PublicClient
	conn      *grpc.ClientConn
	l         log.Logger
}

// New creates a drand client backed by a GRPC connection.
func New(l log.Logger, address, certPath string, insecure bool, chainHash []byte) (client2.Client, error) {
	var opts []grpc.DialOption
	if certPath != "" {
		creds, err := credentials.NewClientTLSFromFile(certPath, "")
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else if insecure {
		opts = append(opts, grpc.WithTransportCredentials(grpcInsec.NewCredentials()))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})))
	}
	opts = append(opts,
		grpc.WithUnaryInterceptor(grpcProm.UnaryClientInterceptor),
		grpc.WithStreamInterceptor(grpcProm.StreamClientInterceptor),
	)
	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, err
	}

	return &grpcClient{address, chainHash, drand.NewPublicClient(conn), conn, l}, nil
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
func (g *grpcClient) Get(ctx context.Context, round uint64) (client2.Result, error) {
	curr, err := g.client.PublicRand(ctx, &drand.PublicRandRequest{Round: round, Metadata: g.getMetadata()})
	if err != nil {
		return nil, err
	}
	if curr == nil {
		return nil, errors.New("no received randomness - unexpected gPRC response")
	}

	return asRD(curr), nil
}

// Watch returns new randomness as it becomes available.
func (g *grpcClient) Watch(ctx context.Context) <-chan client2.Result {
	stream, err := g.client.PublicRandStream(ctx, &drand.PublicRandRequest{Round: 0, Metadata: g.getMetadata()})
	ch := make(chan client2.Result, 1)
	if err != nil {
		close(ch)
		return ch
	}
	go g.translate(stream, ch)
	return ch
}

// Info returns information about the chain.
func (g *grpcClient) Info(ctx context.Context) (*chain2.Info, error) {
	proto, err := g.client.ChainInfo(ctx, &drand.ChainInfoRequest{Metadata: g.getMetadata()})
	if err != nil {
		return nil, err
	}
	if proto == nil {
		return nil, errors.New("no received group - unexpected gPRC response")
	}
	return chain2.InfoFromProto(proto)
}

func (g *grpcClient) translate(stream drand.Public_PublicRandStreamClient, out chan<- client2.Result) {
	defer close(out)
	for {
		next, err := stream.Recv()
		if err != nil || stream.Context().Err() != nil {
			if stream.Context().Err() == nil {
				g.l.Warnw("", "grpc_client", "public rand stream", "err", err)
			}
			return
		}
		out <- asRD(next)
	}
}

func (g *grpcClient) getMetadata() *common.Metadata {
	return &common.Metadata{ChainHash: g.chainHash}
}

func (g *grpcClient) RoundAt(t time.Time) uint64 {
	ctx, cancel := context.WithTimeout(context.Background(), grpcDefaultTimeout)
	defer cancel()

	info, err := g.client.ChainInfo(ctx, &drand.ChainInfoRequest{Metadata: g.getMetadata()})
	if err != nil {
		return 0
	}
	return chain.CurrentRound(t.Unix(), time.Second*time.Duration(info.Period), info.GenesisTime)
}

// SetLog configures the client log output
func (g *grpcClient) SetLog(l log.Logger) {
	g.l = l
}

// Close tears down the gRPC connection and all underlying connections.
func (g *grpcClient) Close() error {
	return g.conn.Close()
}

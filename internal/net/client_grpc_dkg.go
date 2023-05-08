package net

import (
	"context"

	"google.golang.org/grpc"

	"github.com/drand/drand/protobuf/drand"
)

func (g *grpcClient) Propose(ctx context.Context, p Peer, in *drand.ProposalTerms, _ ...grpc.CallOption) (*drand.EmptyResponse, error) {
	var resp *drand.EmptyResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Propose(ctx, in)
	return resp, err
}

func (g *grpcClient) Abort(ctx context.Context, p Peer, in *drand.AbortDKG, _ ...grpc.CallOption) (*drand.EmptyResponse, error) {
	var resp *drand.EmptyResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Abort(ctx, in)
	return resp, err
}

func (g *grpcClient) Execute(ctx context.Context, p Peer, in *drand.StartExecution, _ ...grpc.CallOption) (*drand.EmptyResponse, error) {
	var resp *drand.EmptyResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Execute(ctx, in)
	return resp, err
}

func (g *grpcClient) Accept(ctx context.Context, p Peer, in *drand.AcceptProposal, _ ...grpc.CallOption) (*drand.EmptyResponse, error) {
	var resp *drand.EmptyResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Accept(ctx, in)
	return resp, err
}

func (g *grpcClient) Reject(ctx context.Context, p Peer, in *drand.RejectProposal, _ ...grpc.CallOption) (*drand.EmptyResponse, error) {
	var resp *drand.EmptyResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Reject(ctx, in)
	return resp, err
}

func (g *grpcClient) BroadcastDKG(ctx context.Context, p Peer, in *drand.DKGPacket, _ ...grpc.CallOption) (*drand.EmptyResponse, error) {
	var resp *drand.EmptyResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.BroadcastDKG(ctx, in)
	return resp, err
}

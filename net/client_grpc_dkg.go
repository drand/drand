package net

import (
	"context"

	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/grpc"
)

func (g *grpcClient) Propose(ctx context.Context, p Peer, in *drand.ProposalTerms, opts ...grpc.CallOption) (*drand.EmptyResponse, error) {
	var resp *drand.EmptyResponse
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Propose(ctx, in)
	return resp, err
}

func (g *grpcClient) Abort(ctx context.Context, p Peer, in *drand.AbortDKG, opts ...grpc.CallOption) (*drand.EmptyResponse, error) {
	var resp *drand.EmptyResponse
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Abort(ctx, in)
	return resp, err
}

func (g *grpcClient) Execute(ctx context.Context, p Peer, in *drand.StartExecution, opts ...grpc.CallOption) (*drand.EmptyResponse, error) {
	var resp *drand.EmptyResponse
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Execute(ctx, in)
	return resp, err
}

func (g *grpcClient) Accept(ctx context.Context, p Peer, in *drand.AcceptProposal, opts ...grpc.CallOption) (*drand.EmptyResponse, error) {
	var resp *drand.EmptyResponse
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Accept(ctx, in)
	return resp, err
}

func (g *grpcClient) Reject(ctx context.Context, p Peer, in *drand.RejectProposal, opts ...grpc.CallOption) (*drand.EmptyResponse, error) {
	var resp *drand.EmptyResponse
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Reject(ctx, in)
	return resp, err
}

func (g *grpcClient) BroadcastDKG(ctx context.Context, p Peer, in *drand.DKGPacket, opts ...grpc.CallOption) (*drand.EmptyResponse, error) {
	var resp *drand.EmptyResponse
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.BroadcastDKG(ctx, in)
	return resp, err
}

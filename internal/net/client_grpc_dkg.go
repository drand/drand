package net

import (
	"context"

	"google.golang.org/grpc"

	"github.com/drand/drand/protobuf/drand"
)

func (g *grpcClient) Command(ctx context.Context, p Peer, in *drand.DKGCommand, _ ...grpc.CallOption) (*drand.EmptyDKGResponse, error) {
	var resp *drand.EmptyDKGResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGControlClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Command(ctx, in)
	return resp, err
}

func (g *grpcClient) Packet(ctx context.Context, p Peer, in *drand.GossipPacket, _ ...grpc.CallOption) (*drand.EmptyDKGResponse, error) {
	var resp *drand.EmptyDKGResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGControlClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.Packet(ctx, in)
	return resp, err
}

func (g *grpcClient) DKGStatus(
	ctx context.Context,
	p Peer,
	in *drand.DKGStatusRequest,
	_ ...grpc.CallOption,
) (*drand.DKGStatusResponse, error) {
	var resp *drand.DKGStatusResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGControlClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.DKGStatus(ctx, in)
	return resp, err
}

func (g *grpcClient) BroadcastDKG(ctx context.Context, p Peer, in *drand.DKGPacket, _ ...grpc.CallOption) (*drand.EmptyDKGResponse, error) {
	var resp *drand.EmptyDKGResponse
	c, err := g.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	client := drand.NewDKGControlClient(c)
	ctx, cancel := g.getTimeoutContext(ctx)
	defer cancel()
	resp, err = client.BroadcastDKG(ctx, in)
	return resp, err
}

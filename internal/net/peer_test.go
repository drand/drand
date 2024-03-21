package net

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

type fakeAddr struct {
	v string
}

func (f *fakeAddr) Network() string {
	return "tcp"
}

func (f *fakeAddr) String() string {
	return f.v
}

func TestRemoteAddress(t *testing.T) {
	type testVector struct {
		peer     string
		header   string
		expected string
	}

	var tvs = []testVector{
		{
			"192.168.0.17",
			"",
			"192.168.0.17",
		},
		{
			"10.0.0",
			"myawesomedns.com",
			"myawesomedns.com",
		},
		{
			"myawesomedns.com",
			"superdns.com",
			"myawesomedns.com",
		},
		{
			"",
			"myawesomedns.com",
			"myawesomedns.com",
		},
	}

	for _, test := range tvs {
		p := &peer.Peer{
			Addr: &fakeAddr{test.peer},
		}
		ctx := peer.NewContext(context.Background(), p)
		if test.header != "" {
			mds := metadata.Pairs("x-real-ip", test.header)
			ctx = metadata.NewIncomingContext(ctx, mds)
		}
		ret := RemoteAddress(ctx)
		require.Equal(t, test.expected, ret)
	}
}

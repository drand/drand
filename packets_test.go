package main

import (
	"testing"

	"github.com/dedis/protobuf"
	"github.com/stretchr/testify/require"
)

func TestPacketsMarshalling(t *testing.T) {
	priv := NewKeyPair("127.0.0.1:6789")
	hello := &Drand{Hello: priv.Public}

	buff, err := protobuf.Encode(hello)
	require.NoError(t, err)

	g2 := pairing.G2()
	drand, err := unmarshal(g2, buff)
	require.NoError(t, err)
	require.NotNil(t, drand.Hello)
}

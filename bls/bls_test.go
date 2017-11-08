package bls

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dedis/kyber/util/random"
)

func TestBLSSig(t *testing.T) {
	sk, pk := NewKeyPair(pairing, random.Stream)
	msg := []byte("hello world")

	sig := Sign(pairing, sk, msg)
	require.Nil(t, Verify(pairing, pk, msg, sig))

	wrongMsg := []byte("evil message")
	require.Error(t, Verify(pairing, pk, msg, wrongMsg))
}

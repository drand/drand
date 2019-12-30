package ecies

import (
	"crypto/sha256"
	"testing"

	"github.com/dedis/drand/key"
	"github.com/stretchr/testify/require"
)

func TestECIES(t *testing.T) {
	msg := []byte("shake that cipher")
	kp := key.NewKeyPair("127.0.0.1")
	h := sha256.New
	cipher, err := Encrypt(key.KeyGroup, h, kp.Public.Key, msg)
	require.Nil(t, err)

	plain, err := Decrypt(key.KeyGroup, h, kp.Key, cipher)
	require.Nil(t, err)
	require.Equal(t, msg, plain)
}

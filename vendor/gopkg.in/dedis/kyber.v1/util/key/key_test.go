package key

import (
	"testing"

	"gopkg.in/dedis/kyber.v1/group/edwards25519"
)

func TestNewKeyPair(t *testing.T) {
	suite := edwards25519.NewAES128SHA256Ed25519()
	keypair := NewKeyPair(suite)
	pub := suite.Point().Mul(keypair.Secret, nil)
	if !pub.Equal(keypair.Public) {
		t.Fatal("Public and private-key don't match")
	}
}

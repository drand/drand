package aes

import (
	"testing"

	"gopkg.in/dedis/kyber.v1/util/test"
)

func TestAES(t *testing.T) {
	test.CipherTest(t, NewCipher128)
}

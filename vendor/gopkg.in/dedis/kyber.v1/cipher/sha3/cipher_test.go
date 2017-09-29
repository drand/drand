package sha3

import (
	"testing"

	"gopkg.in/dedis/kyber.v1/util/test"
)

func TestAES(t *testing.T) {
	test.CipherTest(t, NewCipher224)
}

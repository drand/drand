package aes

import (
	"testing"

	"github.com/dedis/kyber/util/test"
)

func TestAES(t *testing.T) {
	test.CipherTest(t, NewCipher128)
}

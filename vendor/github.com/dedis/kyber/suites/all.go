package suites

import (
	"github.com/dedis/kyber/group/edwards25519"
	"github.com/dedis/kyber/pairing/bn256"
)

func init() {
	register(edwards25519.NewBlakeSHA256Ed25519())
	register(bn256.NewSuite().G1().(Suite))
	register(bn256.NewSuite().G2().(Suite))
	register(bn256.NewSuite().GT().(Suite))

}

// +build vartime

package suites

import (
	"github.com/dedis/kyber/group/curve25519"
	"github.com/dedis/kyber/group/nist"
)

func init() {
	register(curve25519.NewBlakeSHA256Curve25519(false))
	register(curve25519.NewBlakeSHA256Curve25519(true))
	register(nist.NewBlakeSHA256P256())
	register(nist.NewBlakeSHA256QR512())
}

package groupsig

import (
	"github.com/dfinity/go-dfinity-crypto/bls"
)

// Init --
func Init(curve int) {
	err := bls.Init(curve)
	if err != nil {
		panic("groupsig.Init")
	}
	curveOrder.SetString(bls.GetCurveOrder(), 10)
	fieldOrder.SetString(bls.GetFieldOrder(), 10)
	bitLength = curveOrder.BitLen()
}

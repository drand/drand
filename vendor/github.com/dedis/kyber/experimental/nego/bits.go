// +build experimental

package nego

import (
	"math/big"
)

// Scan a bit-field within x starting from bit i,
// either upward or downward to but not including bit j,
// for the first bit with value b.
// Returns the position of the first b-bit found,
// or -1 if every bit in the bit-field is set to 1-b.
func BitScan(x *big.Int, i, j int, b uint) int {

	// XXX could be made a lot more efficient using x.Words()
	inc := 1
	if i > j {
		inc = -1
	}
	for ; i != j; i += inc {
		if x.Bit(i) == b {
			return i
		}
	}
	return -1
}

// Set z to x, but with the bit-field from bit i
// up or down to bit j filled with bit value b.
func BitFill(z, x *big.Int, i, j int, b uint) {
	if z != x {
		z.Set(x)
	}
	inc := 1
	if i > j {
		inc = -1
	}
	for ; i != j; i += inc {
		z.SetBit(z, i, b)
	}
}

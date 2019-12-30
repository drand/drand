// +build nolazy

package bls

import (
	"golang.org/x/sys/cpu"
)

type fp struct {
	mul       func(c, a, b *fe)
	mulAssign func(a, b *fe)
}

func newFp() *fp {

	if cpu.X86.HasBMI2 && !enforceNonBMI2 {
		return &fp{
			mul:       montmul_bmi2,
			mulAssign: montmul_assign_bmi2,
		}
	}
	return &fp{
		mul:       montmul_nobmi2,
		mulAssign: montmul_assign_nobmi2,
	}
}

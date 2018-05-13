// +build !vartime

package edwards25519

// In "func (P *point) Mul", this function is referenced, even when !vartime.
// So we need that something is defined, in order to make the linker happy, even if
// it will never be called.
func geScalarMultVartime(h *extendedGroupElement, a *[32]byte,
	A *extendedGroupElement) {
	panic("geScalarMultVartime should never be called with build tags !vartime")
}

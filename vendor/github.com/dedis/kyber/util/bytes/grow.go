package bytes

// Grow makes room for an additional n bytes at the end of slice s.
// Returns the complete resulting slice, and a sub-slice representing
// the newly-extended n-byte region, which the caller must initialize.
// Simply re-slices s in-place if it already has sufficient capacity;
// otherwise allocates a larger slice and copies the existing portion.
func Grow(s []byte, n int) ([]byte, []byte) {
	l := len(s) // existing length
	nl := l + n // extended length
	if nl > cap(s) {
		ns := make([]byte, nl, (nl+1)*2)
		copy(ns, s)
		s = ns
	}
	return s[:nl], s[l:nl]
}

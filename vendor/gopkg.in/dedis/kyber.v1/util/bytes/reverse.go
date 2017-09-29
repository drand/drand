package bytes

// Reverse copies src into dst in byte-reversed order and returns dst,
// such that src[0] goes into dst[len-1] and vice versa.
// dst and src may be the same slice but otherwise must not overlap.
func Reverse(dst, src []byte) []byte {
	if dst == nil {
		dst = make([]byte, len(src))
	} else if len(src) != len(dst) {
		panic("Reverse requires equal-length slices")
	}
	l := len(dst)
	for i, j := 0, l-1; i < (l+1)/2; {
		dst[i], dst[j] = src[j], src[i]
		i++
		j--
	}
	return dst
}

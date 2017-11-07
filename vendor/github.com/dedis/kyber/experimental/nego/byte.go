// +build experimental

package nego

// Simple reservation byte-mask layout
type byteLayout struct {
	mask []byte
}

func (bl *byteLayout) init(max int) {
	bl.mask = make([]byte, max)
}

func (bl *byteLayout) reserve(lo, hi int, excl bool, obj interface{}) bool {
	max := len(bl.mask)
	if hi > max {
		hi = max
	}

	if excl {
		// See if extent is completely available before modifying
		for k := lo; k < hi; k++ {
			if bl.mask[k] != 0 {
				return false
			}
		}
	}

	// Reserve extent inclusively or exclusively
	gotExcl := true
	for k := lo; k < hi; k++ {
		if bl.mask[k] != 0 {
			gotExcl = false
		}
		bl.mask[k] = 1
	}
	return gotExcl
}

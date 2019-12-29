// Package sign contains useful tools for the different signing algorithms.
package sign

import (
	"errors"
	"fmt"

	"github.com/drand/kyber"
	"github.com/drand/kyber/pairing"
)

// Mask is a bitmask of the participation to a collective signature.
type Mask struct {
	mask    []byte
	publics []kyber.Point
}

// NewMask creates a new mask from a list of public keys. If a key is provided, it
// will set the bit of the key to 1 or return an error if it is not found.
func NewMask(suite pairing.Suite, publics []kyber.Point, myKey kyber.Point) (*Mask, error) {
	m := &Mask{
		publics: publics,
	}
	m.mask = make([]byte, m.Len())

	if myKey != nil {
		for i, key := range publics {
			if key.Equal(myKey) {
				m.SetBit(i, true)
				return m, nil
			}
		}

		return nil, errors.New("key not found")
	}

	return m, nil
}

// Mask returns the bitmask as a byte array.
func (m *Mask) Mask() []byte {
	clone := make([]byte, len(m.mask))
	copy(clone[:], m.mask)
	return clone
}

// Len returns the length of the byte array necessary to store the bitmask.
func (m *Mask) Len() int {
	return (len(m.publics) + 7) / 8
}

// SetMask replaces the current mask by the new one if the length matches.
func (m *Mask) SetMask(mask []byte) error {
	if m.Len() != len(mask) {
		return fmt.Errorf("mismatching mask lengths")
	}

	m.mask = mask
	return nil
}

// SetBit turns on or off the bit at the given index.
func (m *Mask) SetBit(i int, enable bool) error {
	if i >= len(m.publics) || i < 0 {
		return errors.New("index out of range")
	}

	byteIndex := i / 8
	mask := byte(1) << uint(i&7)
	if enable {
		m.mask[byteIndex] ^= mask
	} else {
		m.mask[byteIndex] ^= mask
	}
	return nil
}

// forEachBitEnabled is a helper to iterate over the bits set to 1 in the mask
// and to return the result of the callback only if it is positive.
func (m *Mask) forEachBitEnabled(f func(i, j, n int) int) int {
	n := 0
	for i, b := range m.mask {
		for j := uint(0); j < 8; j++ {
			mm := byte(1) << (j & 7)

			if b&mm != 0 {
				if res := f(i, int(j), n); res >= 0 {
					return res
				}

				n++
			}
		}
	}

	return -1
}

// IndexOfNthEnabled returns the index of the nth enabled bit or -1 if out of bounds.
func (m *Mask) IndexOfNthEnabled(nth int) int {
	return m.forEachBitEnabled(func(i, j, n int) int {
		if n == nth {
			return i*8 + int(j)
		}

		return -1
	})
}

// NthEnabledAtIndex returns the sum of bits set to 1 until the given index. In other
// words, it returns how many bits are enabled before the given index.
func (m *Mask) NthEnabledAtIndex(idx int) int {
	return m.forEachBitEnabled(func(i, j, n int) int {
		if i*8+int(j) == idx {
			return n
		}

		return -1
	})
}

// Publics returns a copy of the list of public keys.
func (m *Mask) Publics() []kyber.Point {
	pubs := make([]kyber.Point, len(m.publics))
	copy(pubs, m.publics)
	return pubs
}

// Participants returns the list of public keys participating.
func (m *Mask) Participants() []kyber.Point {
	pp := []kyber.Point{}
	for i, p := range m.publics {
		byteIndex := i / 8
		mask := byte(1) << uint(i&7)
		if (m.mask[byteIndex] & mask) != 0 {
			pp = append(pp, p)
		}
	}

	return pp
}

// CountEnabled returns the number of bit set to 1
func (m *Mask) CountEnabled() int {
	count := 0
	for i := range m.publics {
		byteIndex := i / 8
		mask := byte(1) << uint(i&7)
		if (m.mask[byteIndex] & mask) != 0 {
			count++
		}
	}
	return count
}

// CountTotal returns the number of potential participants
func (m *Mask) CountTotal() int {
	return len(m.publics)
}

// Merge merges the given mask to the current one only if
// the length matches
func (m *Mask) Merge(mask []byte) error {
	if len(m.mask) != len(mask) {
		return errors.New("mismatching mask length")
	}

	for i := range m.mask {
		m.mask[i] |= mask[i]
	}

	return nil
}

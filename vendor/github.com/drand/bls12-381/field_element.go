package bls

import (
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"math/bits"
)

type fe /***			***/ [6]uint64
type fe2 /**			***/ [2]fe
type fe6 /**			***/ [3]fe2
type fe12 /**			***/ [2]fe6
type lfe /**			***/ [12]uint64
type lfe2 /*			***/ [2]lfe
type lfe6 /*			***/ [3]lfe2
type lfe12 /*			***/ [2]lfe6

func (fe *fe) Bytes() []byte {
	out := make([]byte, 48)
	var a int
	for i := 0; i < 6; i++ {
		a = 48 - i*8
		out[a-1] = byte(fe[i])
		out[a-2] = byte(fe[i] >> 8)
		out[a-3] = byte(fe[i] >> 16)
		out[a-4] = byte(fe[i] >> 24)
		out[a-5] = byte(fe[i] >> 32)
		out[a-6] = byte(fe[i] >> 40)
		out[a-7] = byte(fe[i] >> 48)
		out[a-8] = byte(fe[i] >> 56)
	}
	return out
}

func (fe *fe) FromBytes(in []byte) *fe {
	size := 48
	l := len(in)
	if l >= size {
		l = size
	}
	padded := make([]byte, size)
	copy(padded[size-l:], in[:])
	var a int
	for i := 0; i < 6; i++ {
		a = size - i*8
		fe[i] = uint64(padded[a-1]) | uint64(padded[a-2])<<8 |
			uint64(padded[a-3])<<16 | uint64(padded[a-4])<<24 |
			uint64(padded[a-5])<<32 | uint64(padded[a-6])<<40 |
			uint64(padded[a-7])<<48 | uint64(padded[a-8])<<56
	}
	return fe
}

func (fe *fe) SetBig(a *big.Int) *fe {
	return fe.FromBytes(a.Bytes())
}

func (fe *fe) SetUint(a uint64) *fe {
	fe[0] = a
	fe[1] = 0
	fe[2] = 0
	fe[3] = 0
	fe[4] = 0
	fe[5] = 0
	return fe
}

func (fe *fe) SetString(s string) (*fe, error) {
	if s[:2] == "0x" {
		s = s[2:]
	}
	bytes, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return fe.FromBytes(bytes), nil
}

func (fe *fe) Set(fe2 *fe) *fe {
	fe[0] = fe2[0]
	fe[1] = fe2[1]
	fe[2] = fe2[2]
	fe[3] = fe2[3]
	fe[4] = fe2[4]
	fe[5] = fe2[5]
	return fe
}

func (fe *fe) Big() *big.Int {
	return new(big.Int).SetBytes(fe.Bytes())
}

func (fe fe) String() (s string) {
	for i := 5; i >= 0; i-- {
		s = fmt.Sprintf("%s%16.16x", s, fe[i])
	}
	return "0x" + s
}

func (fe *fe) IsOdd() bool {
	var mask uint64 = 1
	return fe[0]&mask != 0
}

func (fe *fe) IsEven() bool {
	var mask uint64 = 1
	return fe[0]&mask == 0
}

func (fe *fe) IsZero() bool {
	return 0 == fe[0] && 0 == fe[1] && 0 == fe[2] && 0 == fe[3] && 0 == fe[4] && 0 == fe[5]
}

func (fe *fe) IsOne() bool {
	return 1 == fe[0] && 0 == fe[1] && 0 == fe[2] && 0 == fe[3] && 0 == fe[4] && 0 == fe[5]
}

func (fe *fe) Cmp(fe2 *fe) int64 {
	if fe[5] > fe2[5] {
		return 1
	} else if fe[5] < fe2[5] {
		return -1
	}
	if fe[4] > fe2[4] {
		return 1
	} else if fe[4] < fe2[4] {
		return -1
	}
	if fe[3] > fe2[3] {
		return 1
	} else if fe[3] < fe2[3] {
		return -1
	}
	if fe[2] > fe2[2] {
		return 1
	} else if fe[2] < fe2[2] {
		return -1
	}
	if fe[1] > fe2[1] {
		return 1
	} else if fe[1] < fe2[1] {
		return -1
	}
	if fe[0] > fe2[0] {
		return 1
	} else if fe[0] < fe2[0] {
		return -1
	}
	return 0
}

func (fe *fe) Equals(fe2 *fe) bool {
	return fe2[0] == fe[0] && fe2[1] == fe[1] && fe2[2] == fe[2] && fe2[3] == fe[3] && fe2[4] == fe[4] && fe2[5] == fe[5]
}

func (fe *fe) div2(e uint64) {
	fe[0] = fe[0]>>1 | fe[1]<<63
	fe[1] = fe[1]>>1 | fe[2]<<63
	fe[2] = fe[2]>>1 | fe[3]<<63
	fe[3] = fe[3]>>1 | fe[4]<<63
	fe[4] = fe[4]>>1 | fe[5]<<63
	fe[5] = fe[5]>>1 | e<<63
}

func (fe *fe) mul2() uint64 {
	e := fe[5] >> 63
	fe[5] = fe[5]<<1 | fe[4]>>63
	fe[4] = fe[4]<<1 | fe[3]>>63
	fe[3] = fe[3]<<1 | fe[2]>>63
	fe[2] = fe[2]<<1 | fe[1]>>63
	fe[1] = fe[1]<<1 | fe[0]>>63
	fe[0] = fe[0] << 1
	return e
}

func (fe *fe) bit(i int) bool {
	k := i >> 6
	i = i - k<<6
	b := (fe[k] >> uint(i)) & 1
	return b != 0
}

func (fe *fe) bitLen() int {
	for i := len(fe) - 1; i >= 0; i-- {
		if len := bits.Len64(fe[i]); len != 0 {
			return len + 64*i
		}
	}
	return 0
}

func (f *fe) rand(max *fe, r io.Reader) error {
	bitLen := bits.Len64(max[5]) + (6-1)*64
	k := (bitLen + 7) / 8
	b := uint(bitLen % 8)
	if b == 0 {
		b = 8
	}
	bytes := make([]byte, k)
	for {
		_, err := io.ReadFull(r, bytes)
		if err != nil {
			return err
		}
		bytes[0] &= uint8(int(1<<b) - 1)
		f.FromBytes(bytes)
		if f.Cmp(max) < 0 {
			break
		}
	}
	return nil
}

func (fe *lfe) Bytes() []byte {
	out := make([]byte, 96)
	var a int
	for i := 0; i < 12; i++ {
		a = 96 - i*8
		out[a-1] = byte(fe[i])
		out[a-2] = byte(fe[i] >> 8)
		out[a-3] = byte(fe[i] >> 16)
		out[a-4] = byte(fe[i] >> 24)
		out[a-5] = byte(fe[i] >> 32)
		out[a-6] = byte(fe[i] >> 40)
		out[a-7] = byte(fe[i] >> 48)
		out[a-8] = byte(fe[i] >> 56)
	}
	return out
}

func (fe *lfe) FromBytes(in []byte) *lfe {
	size := 96
	l := len(in)
	if l >= size {
		l = size
	}
	padded := make([]byte, size)
	copy(padded[size-l:], in[:])
	var a int
	for i := 0; i < 12; i++ {
		a = size - i*8
		fe[i] = uint64(padded[a-1]) | uint64(padded[a-2])<<8 |
			uint64(padded[a-3])<<16 | uint64(padded[a-4])<<24 |
			uint64(padded[a-5])<<32 | uint64(padded[a-6])<<40 |
			uint64(padded[a-7])<<48 | uint64(padded[a-8])<<56
	}
	return fe
}

func (fe lfe) String() (s string) {
	for i := 12; i >= 0; i-- {
		s = fmt.Sprintf("%s%16.16x", s, fe[i])
	}
	return "0x" + s
}

func (fe *lfe) Set(fe2 *lfe) {
	fe[0] = fe2[0]
	fe[1] = fe2[1]
	fe[2] = fe2[2]
	fe[3] = fe2[3]
	fe[4] = fe2[4]
	fe[5] = fe2[5]
	fe[6] = fe2[6]
	fe[7] = fe2[7]
	fe[8] = fe2[8]
	fe[9] = fe2[9]
	fe[10] = fe2[10]
	fe[11] = fe2[11]
}

func (fe *lfe) SetSingle(fe2 *fe) {
	fe[0] = fe2[0]
	fe[1] = fe2[1]
	fe[2] = fe2[2]
	fe[3] = fe2[3]
	fe[4] = fe2[4]
	fe[5] = fe2[5]
	fe[6] = 0
	fe[7] = 0
	fe[8] = 0
	fe[9] = 0
	fe[10] = 0
	fe[11] = 0
}

func (fe *lfe) Equals(fe2 *lfe) bool {
	return fe2[0] == fe[0] && fe2[1] == fe[1] && fe2[2] == fe[2] && fe2[3] == fe[3] && fe2[4] == fe[4] && fe2[5] == fe[5] &&
		fe2[6] == fe[6] && fe2[7] == fe[7] && fe2[8] == fe[8] && fe2[9] == fe[9] && fe2[10] == fe[10] && fe2[11] == fe[11]
}

func (fe fe2) String() (s string) {
	return fe[0].String() + "\n" + fe[1].String()
}

func (fe lfe2) String() (s string) {
	return fe[0].String() + "\n" + fe[1].String()
}

func (fe fe6) String() (s string) {
	return fe[0].String() + "\n" + fe[1].String() + "\n" + fe[2].String()
}

func (fe lfe6) String() (s string) {
	return fe[0].String() + "\n" + fe[1].String() + "\n" + fe[2].String()
}

func (fe fe12) String() (s string) {
	return fe[0].String() + "\n" + fe[1].String()
}

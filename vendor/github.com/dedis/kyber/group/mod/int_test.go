package mod

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntEndianness(t *testing.T) {
	modulo := big.NewInt(65535)
	var v int64 = 65500
	// Let's assume it is bigendian and test that
	i := new(Int).Init64(v, modulo)
	assert.Equal(t, i.BO, BigEndian)

	buff1, err := i.MarshalBinary()
	assert.Nil(t, err)
	i.BO = BigEndian
	buff2, err := i.MarshalBinary()
	assert.Nil(t, err)
	assert.Equal(t, buff1, buff2)

	// Let's change endianness and check the result
	i.BO = LittleEndian
	buff3, err := i.MarshalBinary()
	assert.NotEqual(t, buff2, buff3)

	// let's try LittleEndian function
	buff4 := i.LittleEndian(0, 32)
	assert.Equal(t, buff3, buff4)
	// set endianess but using littleendian should not change anything
	i.BO = BigEndian
	assert.Equal(t, buff4, i.LittleEndian(0, 32))

	// Try to reconstruct the int from the buffer
	i = new(Int).Init64(v, modulo)
	i2 := NewInt64(0, modulo)
	buff, _ := i.MarshalBinary()
	assert.Nil(t, i2.UnmarshalBinary(buff))
	assert.True(t, i.Equal(i2))

	i.BO = LittleEndian
	buff, _ = i.MarshalBinary()
	i2.BO = LittleEndian
	assert.Nil(t, i2.UnmarshalBinary(buff))
	assert.True(t, i.Equal(i2))

	i2.BO = BigEndian
	assert.Nil(t, i2.UnmarshalBinary(buff))
	assert.False(t, i.Equal(i2))
}
func TestIntEndianBytes(t *testing.T) {
	modulo, err := hex.DecodeString("1000")
	moduloI := new(big.Int).SetBytes(modulo)
	assert.Nil(t, err)
	v, err := hex.DecodeString("10")
	assert.Nil(t, err)

	i := new(Int).InitBytes(v, moduloI, BigEndian)

	assert.Equal(t, 2, i.MarshalSize())
	assert.NotPanics(t, func() { i.LittleEndian(2, 2) })
}

func TestInits(t *testing.T) {
	i1 := NewInt64(int64(65500), big.NewInt(65535))
	i2 := NewInt(&i1.V, i1.M)
	assert.True(t, i1.Equal(i2))
	i3 := NewIntBytes(i1.Bytes(), i1.M, BigEndian)
	assert.True(t, i1.Equal(i3))
	i4 := NewIntString(i1.String(), "", 16, i1.M)
	assert.True(t, i1.Equal(i4))
}

func TestIntClone(t *testing.T) {
	moduloI := new(big.Int).SetBytes([]byte{0x10, 0})
	base := new(Int).InitBytes([]byte{0x10}, moduloI, BigEndian)

	clone := base.Clone()
	clone.Add(clone, clone)
	if bytes.Compare(clone.Bytes(), base.Bytes()) == 0 {
		t.Error("Should not be equal")
	}
}

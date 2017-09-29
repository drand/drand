package groupsig

import (
	"log"
	"math/big"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

// ID -- id for secret sharing, represented by big.Int
type ID struct {
	//	value big.Int
	value bls.ID
}

// IsEqual --
func (id ID) IsEqual(rhs ID) bool {
	// TODO : add IsEqual to bls.ID
	return id.value.GetHexString() == rhs.value.GetHexString()
}

// Setters

// SetBigInt --
func (id *ID) SetBigInt(b *big.Int) error {
	return id.value.SetHexString(b.Text(16))
}

// SetDecimalString --
func (id *ID) SetDecimalString(s string) error {
	return id.value.SetDecString(s)
}

// SetHexString --
func (id *ID) SetHexString(s string) error {
	return id.value.SetHexString(s)
}

// Deserialize --
func (id *ID) Deserialize(b []byte) error {
	return id.value.SetLittleEndian(b)
}

// Getters

// GetBigInt --
func (id ID) GetBigInt() *big.Int {
	x := new(big.Int)
	x.SetString(id.value.GetHexString(), 16)
	return x
}

// GetDecimalString --
func (id ID) GetDecimalString() string {
	return id.value.GetDecString()
}

// GetHexString --
func (id ID) GetHexString() string {
	return id.value.GetHexString()
}

// Serialize --
func (id ID) Serialize() []byte {
	return id.value.GetLittleEndian()
}

// Constructors

// NewIDFromBigInt --
func NewIDFromBigInt(b *big.Int) *ID {
	id := new(ID)
	err := id.value.SetDecString(b.Text(10))
	if err != nil {
		log.Printf("NewIDFromBigInt %s\n", err)
		return nil
	}
	return id
}

// NewIDFromInt64 --
func NewIDFromInt64(i int64) *ID {
	return NewIDFromBigInt(big.NewInt(i))
}

// NewIDFromInt --
func NewIDFromInt(i int) *ID {
	return NewIDFromBigInt(big.NewInt(int64(i)))
}

/*
// NewIDFromAddress --
func NewIDFromAddress(addr common.Address) *ID {
	return NewIDFromBigInt(addr.Big())
}
*/

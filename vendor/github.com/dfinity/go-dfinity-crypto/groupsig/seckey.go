package groupsig

import (
	"log"
	"math/big"
	"unsafe"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/dfinity/go-dfinity-crypto/rand"
)

// Curve and Field order
var curveOrder = new(big.Int)
var fieldOrder = new(big.Int)
var bitLength int

// types

// Seckey -- represented by a big.Int modulo curveOrder
type Seckey struct {
	value bls.SecretKey
}

// IsEqual --
func (sec Seckey) IsEqual(rhs Seckey) bool {
	return sec.value.IsEqual(&rhs.value)
}

/*
// SeckeyMap -- a map from addresses to Seckey
type SeckeyMap map[common.Address]Seckey
*/

// SeckeyMapInt -- a map from addresses to Seckey
type SeckeyMapInt map[int]Seckey

// Getters

// Serialize --
func (sec Seckey) Serialize() []byte {
	return sec.value.GetLittleEndian()
}

// GetBigInt --
func (sec Seckey) GetBigInt() (s *big.Int) {
	s = new(big.Int)
	s.SetString(sec.GetHexString(), 16)
	return s
}

// GetDecimalString -- returns a decimal string
func (sec Seckey) GetDecimalString() string {
	return sec.value.GetDecString()
}

// GetHexString -- return hex string without the 0x prefix
func (sec Seckey) GetHexString() string {
	return sec.value.GetHexString()
}

// Constructors

// Deserialize -- check b strictly
func (sec *Seckey) Deserialize(b []byte) error {
	return sec.value.SetLittleEndian(b)
}

// SetLittleEndian -- extend or truncate b to fit in bitLength
func (sec *Seckey) SetLittleEndian(b []byte) error {
	return sec.value.SetLittleEndian(b)
}

// SetHexString --
func (sec *Seckey) SetHexString(s string) error {
	return sec.value.SetHexString(s)
}

// SetDecimalString --
func (sec *Seckey) SetDecimalString(s string) error {
	return sec.value.SetDecString(s)
}

// NewSeckeyFromLittleEndian --
func NewSeckeyFromLittleEndian(b []byte) *Seckey {
	sec := new(Seckey)
	err := sec.SetLittleEndian(b)
	if err != nil {
		log.Printf("NewSeckeyFromLittleEndian %s\n", err)
		return nil
	}
	return sec
}

// NewSeckeyFromRand --
func NewSeckeyFromRand(seed rand.Rand) *Seckey {
	return NewSeckeyFromLittleEndian(seed.Bytes())
}

// NewSeckeyFromBigInt -- use (b mod curveOrder) then no error
func NewSeckeyFromBigInt(b *big.Int) *Seckey {
	b.Mod(b, curveOrder)
	sec := new(Seckey)
	err := sec.value.SetDecString(b.Text(10))
	if err != nil {
		log.Printf("NewSeckeyFromBigInt %s\n", err)
		return nil
	}
	return sec
}

// NewSeckeyFromInt64 --
func NewSeckeyFromInt64(i int64) *Seckey {
	return NewSeckeyFromBigInt(big.NewInt(i))
}

// NewSeckeyFromInt --
func NewSeckeyFromInt(i int) *Seckey {
	return NewSeckeyFromBigInt(big.NewInt(int64(i)))
}

// TrivialSeckey --
func TrivialSeckey() *Seckey {
	return NewSeckeyFromInt64(1)
}

// AggregateSeckeys -- Aggregate multiple seckeys into one by summing up
func AggregateSeckeys(secs []Seckey) *Seckey {
	if len(secs) == 0 {
		log.Printf("AggregateSeckeys no secs")
		return nil
	}
	sec := new(Seckey)
	sec.value = secs[0].value
	for i := 1; i < len(secs); i++ {
		sec.value.Add(&secs[i].value)
	}
	return sec
}

// ShareSeckey -- Derive shares from master through polynomial substitution
func ShareSeckey(msec []Seckey, id ID) *Seckey {
	// #nosec
	msk := *(*[]bls.SecretKey)(unsafe.Pointer(&msec))
	sec := new(Seckey)
	err := sec.value.Set(msk, &id.value)
	if err != nil {
		log.Printf("ShareSeckey err=%s id=%s\n", err, id.GetHexString())
		return nil
	}
	return sec
}

/*
// ShareSeckeyByAddr -- wrapper around sharing by ID
func ShareSeckeyByAddr(msec []Seckey, addr common.Address) *Seckey {
	id := NewIDFromAddress(addr)
	if id == nil {
		log.Printf("ShareSeckeyByAddr bad addr=%s\n", addr)
		return nil
	}
	return ShareSeckey(msec, *id)
}
*/

// ShareSeckeyByInt -- wrapper around sharing by ID
func ShareSeckeyByInt(msec []Seckey, i int) *Seckey {
	return ShareSeckey(msec, *NewIDFromInt64(int64(i)))
}

// ShareSeckeyByMembershipNumber -- wrapper around sharing by ID
func ShareSeckeyByMembershipNumber(msec []Seckey, id int) *Seckey {
	return ShareSeckey(msec, *NewIDFromInt64(int64(id + 1)))
}

// RecoverSeckey -- Recover master from shares through Lagrange interpolation
func RecoverSeckey(secs []Seckey, ids []ID) *Seckey {
	// #nosec
	secVec := *(*[]bls.SecretKey)(unsafe.Pointer(&secs))
	// #nosec
	idVec := *(*[]bls.ID)(unsafe.Pointer(&ids))
	sec := new(Seckey)
	err := sec.value.Recover(secVec, idVec)
	if err != nil {
		log.Printf("RecoverSeckey err=%s\n", err)
		return nil
	}
	return sec
}

/*
// RecoverSeckeyByMap --
func RecoverSeckeyByMap(m SeckeyMap, k int) *Seckey {
	ids := make([]ID, k)
	secs := make([]Seckey, k)
	i := 0
	for a, s := range m {
		id := NewIDFromAddress(a)
		if id == nil {
			log.Printf("RecoverSeckeyByMap bad Address %s\n", a)
			return nil
		}
		ids[i] = *id
		secs[i] = s
		i++
		if i >= k {
			break
		}
	}
	return RecoverSeckey(secs, ids)
}
*/

// RecoverSeckeyByMapInt --
func RecoverSeckeyByMapInt(m SeckeyMapInt, k int) *Seckey {
	ids := make([]ID, k)
	secs := make([]Seckey, k)
	i := 0
	for a, s := range m {
		ids[i] = *NewIDFromInt64(int64(a))
		secs[i] = s
		i++
		if i >= k {
			break
		}
	}
	return RecoverSeckey(secs, ids)
}

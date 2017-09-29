package groupsig

import (
	"log"
	"unsafe"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

// types

// Pubkey -
type Pubkey struct {
	value bls.PublicKey
}

/*
// PubkeyMap --
type PubkeyMap map[common.Address]Pubkey
*/

// IsEqual --
func (pub Pubkey) IsEqual(rhs Pubkey) bool {
	return pub.value.IsEqual(&rhs.value)
}

// Getters

// Deserialize --
func (pub *Pubkey) Deserialize(b []byte) error {
	return pub.value.Deserialize(b)
}

// Serialize --
func (pub Pubkey) Serialize() []byte {
	return pub.value.Serialize()
}

/*
// GetAddress --
func (pub Pubkey) GetAddress() common.Address {
	h := sha3.Sum256(pub.Serialize())

	return common.BytesToAddress(h[:])
}
*/

// GetHexString -- return hex string without the 0x prefix
func (pub Pubkey) GetHexString() string {
	return pub.value.GetHexString()
}

// SetHexString --
func (pub *Pubkey) SetHexString(s string) error {
	return pub.value.SetHexString(s)
}

// Generation

// NewPubkeyFromSeckey -- derive the pubkey from seckey
func NewPubkeyFromSeckey(sec Seckey) *Pubkey {
	pub := new(Pubkey)
	pub.value = *sec.value.GetPublicKey()
	return pub
}

// TrivialPubkey --
func TrivialPubkey() *Pubkey {
	return NewPubkeyFromSeckey(*TrivialSeckey())
}

// AggregatePubkeys -- aggregate multiple into one by summing up
func AggregatePubkeys(pubs []Pubkey) *Pubkey {
	if len(pubs) == 0 {
		log.Printf("AggregatePubkeys no pubs")
		return nil
	}
	pub := new(Pubkey)
	pub.value = pubs[0].value
	for i := 1; i < len(pubs); i++ {
		pub.value.Add(&pubs[i].value)
	}
	return pub
}

// SharePubkey -- Derive shares from master through polynomial substitution
func SharePubkey(mpub []Pubkey, id ID) *Pubkey {
	// #nosec
	mpk := *(*[]bls.PublicKey)(unsafe.Pointer(&mpub))

	pub := new(Pubkey)
	err := pub.value.Set(mpk, &id.value)
	if err != nil {
		log.Printf("SharePubkey err=%s id=%s\n", err, id.GetHexString())
		return nil
	}
	return pub
}

// SharePubkeyByInt -- Wrapper around SharePubkey
func SharePubkeyByInt(mpub []Pubkey, i int) *Pubkey {
	return SharePubkey(mpub, *NewIDFromInt(i))
}

// SharePubkeyByMembershipNumber -- Wrapper around SharePubkey
func SharePubkeyByMembershipNumber(mpub []Pubkey, id int) *Pubkey {
	return SharePubkey(mpub, *NewIDFromInt(id + 1))
}

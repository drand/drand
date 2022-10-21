package client

import "github.com/drand/drand/protobuf/drand"

// RandomData holds the full random response from the server, including data needed
// for validation.
type RandomData struct {
	Rnd               uint64 `json:"round,omitempty"`
	Random            []byte `json:"randomness,omitempty"`
	Sig               []byte `json:"signature,omitempty"`
	PreviousSignature []byte `json:"previous_signature,omitempty"`
}

// Round provides access to the round associatted with this random data.
func (r RandomData) Round() uint64 {
	return r.Rnd
}

// Signature provides the signature over this round's randomness
func (r RandomData) Signature() []byte {
	return r.Sig
}

// Randomness exports the randomness
func (r RandomData) Randomness() []byte {
	return r.Random
}

// Copy generates a copy of the data
func (r RandomData) Copy() RandomData {
	data := RandomData{
		Rnd:               r.Rnd,
		Random:            make([]byte, len(r.Random)),
		Sig:               make([]byte, len(r.Sig)),
		PreviousSignature: make([]byte, len(r.PreviousSignature)),
	}

	copy(data.Random, r.Random)
	copy(data.Sig, r.Sig)
	copy(data.PreviousSignature, r.PreviousSignature)

	return data
}

// FromPublicRandResponse converts the data from the protobuf to the internal type
// It copies everything to ensure data can be independently used.
func FromPublicRandResponse(r *drand.PublicRandResponse) RandomData {
	data := RandomData{
		Rnd:               r.Round,
		Random:            make([]byte, len(r.Randomness)),
		Sig:               make([]byte, len(r.Signature)),
		PreviousSignature: make([]byte, len(r.PreviousSignature)),
	}

	copy(data.Random, r.Randomness)
	copy(data.Sig, r.Signature)
	copy(data.PreviousSignature, r.PreviousSignature)

	return data
}

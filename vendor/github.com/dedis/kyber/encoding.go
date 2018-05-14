package kyber

import (
	"crypto/cipher"
	"encoding"
	"io"
)

/*
Marshaling is a basic interface representing fixed-length (or known-length)
cryptographic objects or structures having a built-in binary encoding.
*/
type Marshaling interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler

	// String returns the human readable string representation of the object.
	String() string

	// Encoded length of this object in bytes.
	MarshalSize() int

	// Encode the contents of this object and write it to an io.Writer.
	MarshalTo(w io.Writer) (int, error)

	// Decode the content of this object by reading from an io.Reader.
	// If r is an XOF, it uses r to pick a valid object pseudo-randomly,
	// which may entail reading more than Len bytes due to retries.
	UnmarshalFrom(r io.Reader) (int, error)
}

/*
Hiding is an alternative encoding interface to encode cryptographic objects
such that their representation appears indistinguishable from a
uniformly random byte-string.

Achieving uniformity in representation is challenging for elliptic curves.
For this reason, the Hiding-encoding of an elliptic curve point
is typically more costly to compute than the normal (non-hidden) encoding,
may be less space efficient,
and may not allow representation for all possible curve points.

This interface allows the ciphersuite to determine
the specific uniform encoding method and balance their tradeoffs.
Since some uniform encodings cannot represent all possible points,
the caller must be prepared to call HideEncode() in a loop
with a freshly-chosen object (typically a fresh Diffie-Hellman public key).

For further background and technical details:

	"Elligator: Elliptic-curve points indistinguishable from uniform random strings"
	http://elligator.cr.yp.to/elligator-20130828.pdf
	"Elligator Squared: Uniform Points on Elliptic Curves of Prime Order as Uniform Random Strings"
	http://eprint.iacr.org/2014/043.pdf
	"Binary Elligator squared"
	http://eprint.iacr.org/2014/486.pdf
*/
type Hiding interface {
	// Hiding-encoded length of this object in bytes.
	HideLen() int

	// Attempt to encode the content of this object into a slice,
	// whose length must be exactly HideLen(), using a specified
	// source of random bits. Encoding may consistently fail on
	// some curve points, in which case this method returns nil,
	// and the caller must try again after re-randomizing the
	// object.
	HideEncode(rand cipher.Stream) []byte

	// Decode a uniform representation of this object from a
	// slice, whose length must be exactly HideLen().  This method
	// cannot fail on correctly-sized input: it maps every
	// HideLen()-byte string to some object.  This is a necessary
	// security property, since if some correctly-sized byte
	// strings failed to decode, an attacker could use decoding as
	// a hidden object detection test.
	HideDecode(buf []byte)
}

// Encoding represents an abstract interface to an encoding/decoding that can be
// used to marshal/unmarshal objects to and from streams. Different Encodings
// will have different constraints, of course. Two implementations are
// available:
//
//   1. The protobuf encoding using the variable length Google Protobuf encoding
//      scheme. The library is available at https://github.com/dedis/protobuf
//   2. The fixbuf encoding, a fixed length binary encoding of arbitrary
//      structures. The library is available at https://github.com/dedis/fixbuf.
type Encoding interface {
	// Encode and write objects to an io.Writer.
	Write(w io.Writer, objs ...interface{}) error

	// Read and decode objects from an io.Reader.
	Read(r io.Reader, objs ...interface{}) error
}

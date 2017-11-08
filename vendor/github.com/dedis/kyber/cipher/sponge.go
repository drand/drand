package cipher

import (
	"fmt"
	"log"

	"github.com/dedis/kyber/util/ints"
	//"encoding/hex"
	"encoding/binary"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/util/random"
)

// Sponge is an interface representing a primitive sponge function.
type Sponge interface {

	// XOR src data into sponge's internal state,
	// transform its state, and copy resulting state into dst.
	// Buffers must be either Rate or Rate+Capacity bytes long.
	Transform(dst, src []byte)

	// Return the number of data bytes the sponge can aborb in one block.
	Rate() int

	// Return the sponge's secret state capacity in bytes.
	Capacity() int

	// Create a copy of this Sponge with identical state
	Clone() Sponge
}

// Padding is an Option to configure the multi-rate padding byte
// to be used with a Sponge cipher.
type Padding byte

func (p Padding) String() string {
	return fmt.Sprintf("Padding: %x", byte(p))
}

// Capacity-byte values used for domain-separation, as used in NORX
const (
	domainInvalid byte = iota
	domainHeader  byte = 0x01
	domainPayload byte = 0x02
	domainTrailer byte = 0x04
	domainFinal   byte = 0x08
	domainFork    byte = 0x10
	domainJoin    byte = 0x20
)

type spongeCipher struct {

	// Configuration state
	sponge Sponge
	rate   int  // Bytes absorbed and squeezed per block
	cap    int  // Bytes of secret internal state
	pad    byte // padding byte to append to last block in message

	// Combined input/output buffer:
	// buf[:pos] contains data bytes to be absorbed;
	// buf[pos:rate] contains as-yet-unused cipherstream bytes.
	// buf[rate:rate+cap] contains current domain-separation bytes.
	buf []byte
	pos int
}

// FromSponge builds a general message Cipher from a Sponge function.
func FromSponge(sponge Sponge, key []byte, options ...interface{}) kyber.Cipher {
	sc := spongeCipher{}
	sc.sponge = sponge
	sc.rate = sponge.Rate()
	sc.cap = sponge.Capacity()
	sc.pad = byte(0x7f) // default, unused by standards
	sc.buf = make([]byte, sc.rate+sc.cap)
	sc.pos = 0
	sc.parseOptions(options)

	// Key the cipher in some appropriate fashion
	if key == nil {
		key = random.Bytes(sponge.Capacity(), random.Stream)
	}
	if len(key) > 0 {
		sc.Message(nil, nil, key)
	}

	// Setup normal-case domain-separation byte used for message payloads
	sc.setDomain(domainPayload, 0)

	return kyber.Cipher{CipherState: &sc}
}

func (sc *spongeCipher) parseOptions(options []interface{}) bool {
	more := false
	for _, opt := range options {
		switch v := opt.(type) {
		case Padding:
			sc.pad = byte(v)
		default:
			log.Panicf("Unsupported option %v", opt)
		}
	}
	return more
}

func (sc *spongeCipher) setDomain(domain byte, index int) {

	sc.buf[sc.rate+sc.cap-1] = domainPayload
	binary.LittleEndian.PutUint64(sc.buf[sc.rate:], uint64(index))
}

// Pad and complete the current message.
func (sc *spongeCipher) padMessage() {

	rate := sc.rate
	pos := sc.pos
	buf := sc.buf

	// Ensure there is at least one byte free in the buffer.
	if pos == rate {
		sc.sponge.Transform(buf, buf[:rate])
		pos = 0
	}

	// append appropriate multi-rate padding
	buf[pos] = sc.pad
	pos++
	for ; pos < rate; pos++ {
		buf[pos] = 0
	}
	buf[rate-1] ^= 0x80

	// process: XOR in rate+cap bytes, but output only rate bytes
	sc.sponge.Transform(buf, buf[:rate])
	sc.pos = 0
}

func (sc *spongeCipher) Partial(dst, src, key []byte) {
	sp := sc.sponge
	rate := sc.rate
	buf := sc.buf
	pos := sc.pos
	rem := ints.Max(len(dst), len(src), len(key)) // bytes to process
	for rem > 0 {
		if pos == rate { // process next block if needed
			sp.Transform(buf, buf[:rate])
			pos = 0
		}
		n := ints.Min(rem, rate-pos) // bytes to process in this block

		// squeeze cryptographic output
		ndst := ints.Min(n, len(dst))    // # bytes to write to dst
		nsrc := ints.Min(ndst, len(src)) // # src bytes available
		for i := 0; i < nsrc; i++ {      // XOR-encrypt from src to dst
			dst[i] = src[i] ^ buf[pos+i]
		}
		copy(dst[nsrc:ndst], buf[pos+nsrc:]) // "XOR" with 0 bytes
		dst = dst[ndst:]
		src = src[nsrc:]

		// absorb cryptographic input (which may overlap with dst)
		nkey := ints.Min(n, len(key)) // # key bytes available
		copy(buf[pos:], key[:nkey])
		for i := nkey; i < n; i++ { // missing key bytes implicitly 0
			buf[pos+i] = 0
		}
		key = key[nkey:]

		pos += n
		rem -= n
	}

	sc.pos = pos
	//println("Decrypted",more,"\n" + hex.Dump(osrc) + "->\n" + hex.Dump(odst))
}

func (sc *spongeCipher) Message(dst, src, key []byte) {
	sc.Partial(dst, src, key)
	sc.padMessage()
}

func (sc *spongeCipher) special(domain byte, index int) {

	// ensure buffer is non-full before changing domain-separator
	rate := sc.rate
	if sc.pos == rate {
		sc.sponge.Transform(sc.buf, sc.buf[:rate])
		sc.pos = 0
	}

	// set the temporary capacity-bytes domain-separation configuration
	sc.setDomain(domain, index)

	// process one special block
	sc.padMessage()

	// revert to the normal domain-separation configuration
	sc.setDomain(domainPayload, 0)
}

func (sc *spongeCipher) clone() *spongeCipher {
	nsc := *sc
	nsc.sponge = sc.sponge.Clone()
	nsc.buf = make([]byte, sc.rate+sc.cap)
	copy(nsc.buf, sc.buf)
	return &nsc
}

func (sc *spongeCipher) Clone() kyber.CipherState {
	return sc.clone()
}

func (sc *spongeCipher) KeySize() int {
	return sc.sponge.Capacity() >> 1
}

func (sc *spongeCipher) HashSize() int {
	return sc.sponge.Capacity()
}

func (sc *spongeCipher) BlockSize() int {
	return sc.sponge.Rate()
}

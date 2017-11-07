package bench

import (
	c "crypto/cipher"
	"crypto/rc4"
	"testing"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/cipher"
	"github.com/dedis/kyber/cipher/aes"
	"github.com/dedis/kyber/cipher/norx"
	"github.com/dedis/kyber/cipher/sha3"
	"golang.org/x/crypto/blowfish"
	"golang.org/x/crypto/salsa20"
	"golang.org/x/crypto/twofish"
)

var buf = make([]byte, 1024*1024)

// benchmarkCipher tests the speed of a Cipher to process a size-byte message.
func benchmarkCipher(b *testing.B, cipher kyber.Cipher, size int) {
	b.SetBytes(int64(size))
	for i := 0; i < b.N; i++ {
		cipher.Message(buf[:size], buf[:size], buf[:size])
	}
}

// 1B messages

func BenchmarkAes128_1B(b *testing.B) {
	benchmarkCipher(b, aes.NewCipher128(cipher.NoKey), 1)
}
func BenchmarkAes192_1B(b *testing.B) {
	benchmarkCipher(b, aes.NewCipher192(cipher.NoKey), 1)
}
func BenchmarkAes256_1B(b *testing.B) {
	benchmarkCipher(b, aes.NewCipher256(cipher.NoKey), 1)
}

func BenchmarkShake128_1B(b *testing.B) {
	benchmarkCipher(b, sha3.NewShakeCipher128(cipher.NoKey), 1)
}
func BenchmarkShake256_1B(b *testing.B) {
	benchmarkCipher(b, sha3.NewShakeCipher256(cipher.NoKey), 1)
}
func BenchmarkSha3_224_1B(b *testing.B) {
	benchmarkCipher(b, sha3.NewCipher224(cipher.NoKey), 1)
}
func BenchmarkSha3_256_1B(b *testing.B) {
	benchmarkCipher(b, sha3.NewCipher256(cipher.NoKey), 1)
}
func BenchmarkSha3_384_1B(b *testing.B) {
	benchmarkCipher(b, sha3.NewCipher384(cipher.NoKey), 1)
}
func BenchmarkSha3_512_1B(b *testing.B) {
	benchmarkCipher(b, sha3.NewCipher512(cipher.NoKey), 1)
}

func BenchmarkNORX_1B(b *testing.B) {
	benchmarkCipher(b, norx.NewCipher(cipher.NoKey), 1)
}

// 1K messages

func BenchmarkAes128_1K(b *testing.B) {
	benchmarkCipher(b, aes.NewCipher128(cipher.NoKey), 1024)
}
func BenchmarkAes192_1K(b *testing.B) {
	benchmarkCipher(b, aes.NewCipher192(cipher.NoKey), 1024)
}
func BenchmarkAes256_1K(b *testing.B) {
	benchmarkCipher(b, aes.NewCipher256(cipher.NoKey), 1024)
}

func BenchmarkShake128_1K(b *testing.B) {
	benchmarkCipher(b, sha3.NewShakeCipher128(cipher.NoKey), 1024)
}
func BenchmarkShake256_1K(b *testing.B) {
	benchmarkCipher(b, sha3.NewShakeCipher256(cipher.NoKey), 1024)
}
func BenchmarkSha3_224_1K(b *testing.B) {
	benchmarkCipher(b, sha3.NewCipher224(cipher.NoKey), 1024)
}
func BenchmarkSha3_256_1K(b *testing.B) {
	benchmarkCipher(b, sha3.NewCipher256(cipher.NoKey), 1024)
}
func BenchmarkSha3_384_1K(b *testing.B) {
	benchmarkCipher(b, sha3.NewCipher384(cipher.NoKey), 1024)
}
func BenchmarkSha3_512_1K(b *testing.B) {
	benchmarkCipher(b, sha3.NewCipher512(cipher.NoKey), 1024)
}

func BenchmarkNORX_1K(b *testing.B) {
	benchmarkCipher(b, norx.NewCipher(cipher.NoKey), 1024)
}

// 1M messages

/*
func BenchmarkAes128_1M(b *testing.B) {
	benchmarkCipher(b, aes.NewCipher128(cipher.NoKey), 1024*1024)
}
func BenchmarkAes192_1M(b *testing.B) {
	benchmarkCipher(b, aes.NewCipher192(cipher.NoKey), 1024*1024)
}
func BenchmarkAes256_1M(b *testing.B) {
	benchmarkCipher(b, aes.NewCipher256(cipher.NoKey), 1024*1024)
}
*/

func BenchmarkShake128_1M(b *testing.B) {
	benchmarkCipher(b, sha3.NewShakeCipher128(cipher.NoKey), 1024*1024)
}
func BenchmarkShake256_1M(b *testing.B) {
	benchmarkCipher(b, sha3.NewShakeCipher256(cipher.NoKey), 1024*1024)
}
func BenchmarkSha3_224_1M(b *testing.B) {
	benchmarkCipher(b, sha3.NewCipher224(cipher.NoKey), 1024*1024)
}
func BenchmarkSha3_256_1M(b *testing.B) {
	benchmarkCipher(b, sha3.NewCipher256(cipher.NoKey), 1024*1024)
}
func BenchmarkSha3_384_1M(b *testing.B) {
	benchmarkCipher(b, sha3.NewCipher384(cipher.NoKey), 1024*1024)
}
func BenchmarkSha3_512_1M(b *testing.B) {
	benchmarkCipher(b, sha3.NewCipher512(cipher.NoKey), 1024*1024)
}

func BenchmarkNORX_1M(b *testing.B) {
	benchmarkCipher(b, norx.NewCipher(cipher.NoKey), 1024*1024)
}

// Some conventional Stream ciphers for comparison

func benchmarkStream(b *testing.B, stream cipher.Stream, size int) {
	b.SetBytes(int64(size))
	for i := 0; i < b.N; i++ {
		stream.XORKeyStream(buf[:size], buf[:size])
	}
}

func benchmarkBlock(b *testing.B, block cipher.Block, testsize int) {
	iv := make([]byte, block.BlockSize())
	stream := c.NewCTR(block, iv)
	benchmarkStream(b, stream, testsize)
}

// AES in CTR mode, the old standby
func BenchmarkAesCtr128_1K(b *testing.B) {
	aes, _ := aes.NewBlockCipher(buf[:16])
	benchmarkBlock(b, aes, 1024)
}

// RC4: obsolete, but for fun
func BenchmarkRc4_1K(b *testing.B) {
	strm, _ := rc4.NewCipher(buf[:1])
	benchmarkStream(b, strm, 1024)
}

// Salsa20 cipher
func BenchmarkSalsa20_1K(b *testing.B) {
	var key [32]byte
	var nonce [8]byte
	size := 1024
	b.SetBytes(int64(size))
	for i := 0; i < b.N; i++ {
		salsa20.XORKeyStream(buf[:size], buf[:size], nonce[:], &key)
	}
}

// Blowfish: obsolete, but for fun
func BenchmarkBlowfish_1K(b *testing.B) {
	block, _ := blowfish.NewCipher(buf[:32])
	benchmarkBlock(b, block, 1024)
}

// Twofish cipher
func BenchmarkTwofish_1K(b *testing.B) {
	block, _ := twofish.NewCipher(buf[:16])
	benchmarkBlock(b, block, 1024)
}

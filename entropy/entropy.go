package entropy

import (
	"crypto/rand"
)

// CustomRandomSource is a generic interface that any drand node operator can implement
// to provide their own source of randomness in function GetRandom.
type EntropySource interface {
	Read(data []byte) (n int, err error)
}

// GetRandom reads len bytes of randomness from whatever Reader is passed in, and returns
// those bytes as the requested randomness.
func GetRandom(source EntropySource, len uint32) ([]byte, error) {
	if source == nil {
		source = rand.Reader
	}

	randomBytes := make([]byte, len)
	bytesRead, err := source.Read(randomBytes)
	if err != nil || uint32(bytesRead) != len {
		// If customEntropy provides an error, fallback to Golang crypto/rand generator.
		_, err := rand.Read(randomBytes)
		return randomBytes, err
	}
	return randomBytes, nil
}

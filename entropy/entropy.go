package entropy

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"errors"
	"os/exec"
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

// EntropyReader hold info for user entropy to be given to the dkg
// Warning: UserOnly should be set to true for debugging, and randomness coming from user
// should always be mixed with crypto/rand and thus userOnly dhoul be false by default
type EntropyReader struct {
	Entropy  string
	UserOnly bool
}

func (r *EntropyReader) Read(p []byte) (n int, err error) {
	if r.Entropy == "" {
		return 0, errors.New("No reader was provided")
	}
	cmd := exec.Command("./" + r.Entropy)
	var b bytes.Buffer
	cmd.Stdout = bufio.NewWriter(&b)

	err = cmd.Run()
	if err != nil {
		return 0, err
	}
	copy(p, b.Bytes())
	return b.Len(), nil
}

// GetEntropy returns the path of the file to use as additional entropy
func (r *EntropyReader) GetEntropy() string {
	return r.Entropy
}

// GetUserOnly returns true if user wants to use their randomness only
func (r *EntropyReader) GetUserOnly() bool {
	return r.UserOnly
}

// NewEntropyReader creates a new EntropyReader struct
func NewEntropyReader(path string, userOnly bool) *EntropyReader {
	return &EntropyReader{path, userOnly}
}

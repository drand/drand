package entropy

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
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
		// If customEntropy provides an error,
		// fallback to Golang crypto/rand generator.
		_, err := rand.Read(randomBytes)
		return randomBytes, err
	}
	return randomBytes, nil
}

// EntropyReader hold info for user entropy to be given to the dkg
type EntropyReader struct {
	Entropy string
}

var _ io.Reader = &EntropyReader{}

// Read calls the executable as many times needed to fill the array p
// n == len(p) if and only if err == nil
func (r *EntropyReader) Read(p []byte) (n int, err error) {
	if r.Entropy == "" {
		return 0, errors.New("No reader was provided")
	}
	var b bytes.Buffer
	read := 0
	for read < len(p) {
		cmd := exec.Command(r.Entropy)
		cmd.Stdout = bufio.NewWriter(&b)
		err = cmd.Run()
		if err != nil {
			fmt.Printf("entropy: cannot read from the file: %s\v", err.Error())
			return read, err
		}
		read += copy(p[read:], b.Bytes())
	}
	return len(p), nil
}

// GetEntropy returns the path of the file to use as additional entropy
func (r *EntropyReader) GetEntropy() string {
	return r.Entropy
}

// NewEntropyReader creates a new EntropyReader struct
func NewEntropyReader(path string) *EntropyReader {
	return &EntropyReader{path}
}

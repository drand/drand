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

// GetRandom reads n bytes of randomness from whatever Reader is passed in, and returns
// those bytes as the requested randomness.
func GetRandom(source io.Reader, n uint32) ([]byte, error) {
	if source == nil {
		source = rand.Reader
	}

	randomBytes := make([]byte, n)
	bytesRead, err := source.Read(randomBytes)
	if err != nil || uint32(bytesRead) != n {
		// If customEntropy provides an error,
		// fallback to Golang crypto/rand generator.
		_, err := rand.Read(randomBytes)
		return randomBytes, err
	}
	return randomBytes, nil
}

// ScriptReader hold info for user entropy to be given to the dkg
type ScriptReader struct {
	Path string
}

var _ io.Reader = &ScriptReader{}

// Read calls the executable as many times needed to fill the array p
// n == len(p) if and only if err == nil
func (r *ScriptReader) Read(p []byte) (n int, err error) {
	if r.Path == "" {
		return 0, errors.New("no reader was provided")
	}
	var b bytes.Buffer
	read := 0
	for read < len(p) {
		cmd := exec.Command(r.Path) // #nosec
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

// GetPath returns the path of the script
func (r *ScriptReader) GetPath() string {
	return r.Path
}

// NewScriptReader creates a new ScriptReader struct
func NewScriptReader(path string) *ScriptReader {
	return &ScriptReader{path}
}

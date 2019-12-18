package entropy

import (
	"crypto/rand"
	"io"
	"os"
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

// GetEntropy returns the path of the file to use as additional entropy
func (r *EntropyReader) GetEntropy() string {
	return r.Entropy
}

// ExtractReader creates an io.Reader using r's entropy file
func (r *EntropyReader) ExtractReader() io.Reader {
	if r.Entropy == "" {
		return nil
	}
	// f should have been tested already XXX again ?
	f, _ := os.Open(r.Entropy)
	return f
}

// GetUserOnly returns true if user wants to use their randomness only
func (r *EntropyReader) GetUserOnly() bool {
	return r.UserOnly
}

// NewEntropyReader creates a new EntropyReader struct
func NewEntropyReader(path string, userOnly bool) *EntropyReader {
	return &EntropyReader{path, userOnly}
}

func CreateFileFromExec(execPath string) (string, error) {
	cmd := exec.Command("./" + execPath)
	outfile, err := os.Create("./entropyCapture.txt")
	if err != nil {
		return "", err
	}
	defer outfile.Close()
	cmd.Stdout = outfile

	err = cmd.Run()
	if err != nil {
		return "", err
	}
	return outfile.Name(), nil
}

package entropy

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"

	"github.com/drand/drand/v2/common/log"
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

// NewFileReader creates a reader that reads random bytes directly from a file
func NewFileReader(filePath string) io.Reader {
	return &fileReader{
		path: filePath,
	}
}

type fileReader struct {
	path string
}

func (r *fileReader) Read(p []byte) (n int, err error) {
	// Open the file for reading
	file, err := os.Open(r.path)
	if err != nil {
		return 0, fmt.Errorf("entropy: cannot open file: %w", err)
	}
	defer file.Close()

	// Read directly from the file
	n, err = file.Read(p)
	if err != nil {
		return 0, fmt.Errorf("entropy: error reading from file: %w", err)
	}

	return n, nil
}

// GetReaderFromSource creates a reader for the provided file path
func GetReaderFromSource(sourcePath string, logger log.Logger) (io.Reader, error) {
	fileInfo, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("entropy: cannot access source: %w", err)
	}

	if fileInfo.IsDir() {
		return nil, fmt.Errorf("entropy: source path is a directory, not a file")
	}

	logger.Infow("Using file for entropy source", "source", sourcePath)
	return NewFileReader(sourcePath), nil
}

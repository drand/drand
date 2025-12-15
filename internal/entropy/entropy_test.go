package entropy

import (
	"bytes"
	"os"
	"testing"

	"github.com/drand/drand/v2/common/log"
	"github.com/stretchr/testify/require"
)

func TestGetRandomness32BytesDefault(t *testing.T) {
	random, err := GetRandom(nil, 32)
	if err != nil {
		t.Fatal("Getting randomness failed:", err)
	}
	if len(random) != 32 {
		t.Fatal("Randomness incorrect number of bytes:", len(random), "instead of 32")
	}
}

func TestNoDuplicatesDefault(t *testing.T) {
	random1, err := GetRandom(nil, 32)
	if err != nil {
		t.Fatal("Getting randomness failed:", err)
	}

	random2, err := GetRandom(nil, 32)
	if err != nil {
		t.Fatal("Getting randomness failed:", err)
	}
	if bytes.Equal(random1, random2) {
		t.Fatal("Randomness was the same for two samples, which is incredibly unlikely")
	}
}

func TestFileReader(t *testing.T) {
	// Create a temporary file with test data
	testData := []byte("test random data for file reader")
	tmpFile, err := os.CreateTemp("", "test-entropy-*.dat")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(testData); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Create a file reader with the temporary file
	reader := NewFileReader(tmpFile.Name())

	// Read from the file and verify the output
	data := make([]byte, len(testData))
	n, err := reader.Read(data)
	if err != nil {
		t.Fatalf("Failed to read from file reader: %v", err)
	}

	if n != len(testData) {
		t.Errorf("Expected to read %d bytes, got %d", len(testData), n)
	}

	if !bytes.Equal(data, testData) {
		t.Errorf("Expected data %q, got %q", testData, data)
	}
}

func TestGetReaderFromSource(t *testing.T) {
	// Create a regular file
	regularData := []byte("regular file data")
	regularFile, err := os.CreateTemp("", "test-regular-*.dat")
	if err != nil {
		t.Fatalf("Failed to create regular temp file: %v", err)
	}
	defer os.Remove(regularFile.Name())

	if _, err := regularFile.Write(regularData); err != nil {
		t.Fatalf("Failed to write to regular file: %v", err)
	}
	if err := regularFile.Close(); err != nil {
		t.Fatalf("Failed to close regular file: %v", err)
	}

	// Create a directory
	tempDir, err := os.MkdirTemp("", "test-dir-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test logger
	logger := log.DefaultLogger().Named("test")

	// Test with regular file
	reader, err := GetReaderFromSource(regularFile.Name(), logger)
	if err != nil {
		t.Fatalf("Failed to get reader for regular file: %v", err)
	}

	_, ok := reader.(*fileReader)
	if !ok {
		t.Errorf("Expected fileReader for regular file, got %T", reader)
	}

	// Test with directory (should fail)
	_, err = GetReaderFromSource(tempDir, logger)
	if err == nil {
		t.Error("Expected error when using directory as source, got nil")
	}

	// Test with non-existent file
	_, err = GetReaderFromSource("/nonexistent/file", logger)
	if err == nil {
		t.Error("Expected error when using non-existent file as source, got nil")
	}
}

func TestGetReaderFromSourceValidFile(t *testing.T) {
	// Create a temporary file with random data
	fileData := []byte("randomdata")
	tmpFile, err := os.CreateTemp("", "test-entropy-*.dat")
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(fileData)
	require.NoError(t, err, "Failed to write to temp file")
	err = tmpFile.Close()
	require.NoError(t, err, "Failed to close temp file")

	logger := log.New(nil, log.DebugLevel, false)
	reader, err := GetReaderFromSource(tmpFile.Name(), logger)
	require.NoError(t, err, "GetReaderFromSource failed for valid file")
	require.NotNil(t, reader, "Reader should not be nil for valid file")

	// Try reading from the reader
	buf := make([]byte, len(fileData))
	n, err := reader.Read(buf)
	require.NoError(t, err, "Reading from file reader failed")
	require.Equal(t, len(fileData), n, "Incorrect number of bytes read")
	require.Equal(t, fileData, buf, "Read data does not match original data")
}

func TestFileReaderWithDevUrandom(t *testing.T) {
	logger := log.New(nil, log.DebugLevel, false)
	reader, err := GetReaderFromSource("/dev/urandom", logger)
	require.NoError(t, err, "GetReaderFromSource failed for /dev/urandom")
	require.NotNil(t, reader, "Reader should not be nil for /dev/urandom")

	// Attempt to read 32 bytes from /dev/urandom
	bufferSize := 32
	buf := make([]byte, bufferSize)
	n, err := reader.Read(buf)

	require.NoError(t, err, "Reading from /dev/urandom failed")
	require.Equal(t, bufferSize, n, "Incorrect number of bytes read from /dev/urandom")

	// Sanity check: ensure the buffer is not all zeros
	allZeros := true
	for _, b := range buf {
		if b != 0 {
			allZeros = false
			break
		}
	}
	require.False(t, allZeros, "Buffer from /dev/urandom should not be all zeros")

	// Attempt a second read to ensure the file is reopened and read correctly
	n2, err2 := reader.Read(buf) // Read into the same buffer to check new random data
	require.NoError(t, err2, "Second read from /dev/urandom failed")
	require.Equal(t, bufferSize, n2, "Incorrect number of bytes read from /dev/urandom on second attempt")

	allZeros2 := true
	for _, b := range buf {
		if b != 0 {
			allZeros2 = false
			break
		}
	}
	require.False(t, allZeros2, "Buffer from /dev/urandom should not be all zeros on second attempt")
}

func TestGetReaderFromSourceNonExistentFile(t *testing.T) {
	logger := log.New(nil, log.DebugLevel, false)
	_, err := GetReaderFromSource("/path/to/nonexistent/file", logger)
	require.Error(t, err, "GetReaderFromSource should fail for non-existent file")
}

func TestGetReaderFromSourceDirectory(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "test-entropy-dir")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tmpDir)

	logger := log.New(nil, log.DebugLevel, false)
	_, err = GetReaderFromSource(tmpDir, logger)
	require.Error(t, err, "GetReaderFromSource should fail for a directory")
	require.Contains(t, err.Error(), "entropy: source path is a directory, not a file")
}

func TestGetReaderFromSourcePermissions(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "test-unreadable-*.dat")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write some data
	_, err = tmpFile.WriteString("some data")
	require.NoError(t, err)
	err = tmpFile.Close()
	require.NoError(t, err)

	// Change permissions to make it unreadable
	err = os.Chmod(tmpFile.Name(), 0000) // No read, no write, no execute
	require.NoError(t, err)

	// Attempt to get a reader (this should succeed, as os.Stat might still work)
	logger := log.New(nil, log.DebugLevel, false)
	reader, err := GetReaderFromSource(tmpFile.Name(), logger)
	require.NoError(t, err) // os.Stat might work even if file is unreadable by user
	require.NotNil(t, reader)

	// Attempt to read from the unreadable file
	buf := make([]byte, 5)
	_, readErr := reader.Read(buf)
	require.Error(t, readErr, "Reading from unreadable file should fail")
	require.Contains(t, readErr.Error(), "entropy: cannot open file")

	// Restore permissions so it can be deleted
	err = os.Chmod(tmpFile.Name(), 0600)
	require.NoError(t, err)
}

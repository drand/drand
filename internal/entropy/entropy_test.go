package entropy

import (
	"bytes"
	"os"
	"testing"

	"github.com/drand/drand/v2/common/log"
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
		t.Fatal("Randomness was the same for two samples, which is incorrect.")
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

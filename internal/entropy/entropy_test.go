package entropy

import (
	"bytes"
	"io"
	"os"
	"testing"

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
		t.Fatal("Randomness was the same for two samples, which is incorrect.")
	}
}

func TestEntropyRead(t *testing.T) {
	file, err := os.Create("./veryrandom.sh")
	require.NoError(t, err)

	require.NoError(t, file.Chmod(0740))

	_, err = file.WriteString("#!/bin/sh\necho Hey, good morning, Monstropolis")
	require.NoError(t, err)

	file.Close()
	t.Cleanup(func() {
		os.Remove("./veryrandom.sh")
	})

	execRand := "./veryrandom.sh"
	entropyReader := NewScriptReader(execRand)
	p := make([]byte, 32)
	n, err := entropyReader.Read(p)
	if err != nil || n != len(p) {
		t.Fatal("read did not work")
	}
}

func TestEntropyReadSmallExec(t *testing.T) {
	file, err := os.Create("./veryrandom2.sh")
	require.NoError(t, err)

	require.NoError(t, file.Chmod(0740))

	_, err = file.WriteString("#!/bin/sh\necho Hey")
	require.NoError(t, err)

	file.Close()
	t.Cleanup(func() {
		os.Remove("./veryrandom2.sh")
	})

	execRand := "./veryrandom2.sh"
	entropyReader := NewScriptReader(execRand)
	p := make([]byte, 32)
	n, err := entropyReader.Read(p)
	if err != nil || n != len(p) {
		t.Fatal("read did not work", n, err)
	}
}

func TestNewScriptReader(t *testing.T) {
	// Create a temporary script file that outputs random bytes
	scriptContent := `#!/bin/sh
echo "randomdata"
`
	tmpFile, err := os.CreateTemp("", "test-script-*.sh")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(scriptContent)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Make the script executable
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		t.Fatalf("Failed to make script executable: %v", err)
	}

	// Create a new script reader with the temporary script
	reader := NewScriptReader(tmpFile.Name())

	// Read from the script and verify the output
	data := make([]byte, 10)
	n, err := reader.Read(data)
	if err != nil && err != io.EOF {
		t.Fatalf("Failed to read from script reader: %v", err)
	}

	expectedData := []byte("randomdata\n")
	if !bytes.Equal(data[:n], expectedData[:n]) {
		t.Errorf("Expected data %q, got %q", expectedData[:n], data[:n])
	}
}

func TestScriptReaderError(t *testing.T) {
	// Test with a non-existent script
	reader := NewScriptReader("/nonexistent/script/path")

	data := make([]byte, 10)
	_, err := reader.Read(data)
	if err == nil {
		t.Error("Expected error when reading from non-existent script, got nil")
	}
}

func TestScriptReaderMultipleReads(t *testing.T) {
	// Create a temporary script file that outputs random bytes
	scriptContent := `#!/bin/sh
echo "morethantenbytes"
`
	tmpFile, err := os.CreateTemp("", "test-script-*.sh")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(scriptContent)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Make the script executable
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		t.Fatalf("Failed to make script executable: %v", err)
	}

	// Create a new script reader
	reader := NewScriptReader(tmpFile.Name())

	// First read
	data1 := make([]byte, 5)
	n1, err := reader.Read(data1)
	if err != nil && err != io.EOF {
		t.Fatalf("Failed on first read: %v", err)
	}

	// ScriptReader runs the script every time it's called,
	// so we should get the beginning of the output each time
	expectedData := []byte("morethantenbytes\n")
	if !bytes.Equal(data1[:n1], expectedData[:n1]) {
		t.Errorf("First read failed. Expected %q, got %q", expectedData[:n1], data1[:n1])
	}

	// Second read - should run the script again
	data2 := make([]byte, 5)
	n2, err := reader.Read(data2)
	if err != nil && err != io.EOF {
		t.Fatalf("Failed on second read: %v", err)
	}

	// Each read should contain the same data, as the script is re-run each time
	if !bytes.Equal(data1[:n1], data2[:n2]) {
		t.Errorf("Expected same data for both reads. First read: %q, Second read: %q",
			data1[:n1], data2[:n2])
	}
}

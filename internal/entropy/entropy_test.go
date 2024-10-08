package entropy

import (
	"bytes"
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

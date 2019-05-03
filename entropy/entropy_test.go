package entropy

import (
	"bytes"
	"testing"
)

func TestGetRandomness32Bytes(t *testing.T) {
	random, err := GetRandom(32)
	if err != nil {
		t.Fatal("Getting lavarand randomness failed:", err)
	}
	if len(random) != 32 {
		t.Fatal("Randomness incorrect number of bytes:", len(random), "instead of 32")
	}
}

func TestNoDuplicates(t *testing.T) {
	random1, err := GetRandom(32)
	if err != nil {
		t.Fatal("Getting lavarand randomness failed:", err)
	}

	random2, err := GetRandom(32)
	if err != nil {
		t.Fatal("Getting lavarand randomness failed:", err)
	}
	if bytes.Compare(random1, random2) == 0 {
		t.Fatal("Randomness was the same for two samples, which is incorrect.")
	}
}

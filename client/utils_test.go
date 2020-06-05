package client

import (
	"bytes"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/test"
)

// fakeChainInfo creates a chain info object for use in tests.
func fakeChainInfo() *chain.Info {
	return &chain.Info{
		Period:      time.Second,
		GenesisTime: time.Now().Unix(),
		PublicKey:   test.GenerateIDs(1)[0].Public.Key,
	}
}

// nextResult reads the next result from the channel and fails the test if it closes before a value is read.
func nextResult(t *testing.T, ch <-chan Result) Result {
	r, ok := <-ch
	if !ok {
		t.Fatal("closed before result")
	}
	return r
}

// compareResults asserts that two results are the same.
func compareResults(t *testing.T, a, b Result) {
	if a.Round() != b.Round() {
		t.Fatal("unexpected result round", a.Round(), b.Round())
	}
	if bytes.Compare(a.Randomness(), b.Randomness()) != 0 {
		t.Fatal("unexpected result randomness", a.Randomness(), b.Randomness())
	}
}

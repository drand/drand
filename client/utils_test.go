package client

import (
	"bytes"
	"context"
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

func latestResult(t *testing.T, c Client) Result {
	r, err := c.Get(context.Background(), 0)
	if err != nil {
		t.Fatal("getting latest result", err)
	}
	return r
}

// nextResult reads the next result from the channel and fails the test if it closes before a value is read.
func nextResult(t *testing.T, ch <-chan Result) Result {
	t.Helper()

	select {
	case r, ok := <-ch:
		if !ok {
			t.Fatal("closed before result")
		}
		return r
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for result.")
		return nil
	}
}

// compareResults asserts that two results are the same.
func compareResults(t *testing.T, a, b Result) {
	t.Helper()

	if a.Round() != b.Round() {
		t.Fatal("unexpected result round", a.Round(), b.Round())
	}
	if !bytes.Equal(a.Randomness(), b.Randomness()) {
		t.Fatal("unexpected result randomness", a.Randomness(), b.Randomness())
	}
}

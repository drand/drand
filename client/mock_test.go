package client

import (
	"context"
	"errors"
	"time"
)

// MockClient provide a mocked client interface
type MockClient struct {
	Results []MockResult
}

// Get returns a the randomness at `round` or an error.
func (m *MockClient) Get(ctx context.Context, round uint64) (Result, error) {
	if len(m.Results) == 0 {
		return nil, errors.New("No result available")
	}
	r := m.Results[0]
	m.Results = m.Results[1:]
	return &r, nil
}

// Watch returns new randomness as it becomes available.
func (m *MockClient) Watch(ctx context.Context) <-chan Result {
	ch := make(chan Result, 1)
	r, _ := m.Get(ctx, 0)
	ch <- r
	close(ch)
	return ch
}

// RoundAt will return the most recent round of randomness
func (m *MockClient) RoundAt(time time.Time) uint64 {
	return 0
}

// ClientWithResults returns a client on which `Get` works `m-n` times.
func MockClientWithResults(n, m int) Client {
	c := new(MockClient)
	for i := n; i < m; i++ {
		c.Results = append(c.Results, MockResult{uint64(i), []byte{byte(i)}})
	}
	return c
}

type MockResult struct {
	rnd  uint64
	rand []byte
}

func (r *MockResult) Randomness() []byte {
	return r.rand
}

func (r *MockResult) Round() uint64 {
	return r.rnd
}

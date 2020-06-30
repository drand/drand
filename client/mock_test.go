package client

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client/test/result/mock"
)

// MockClient provide a mocked client interface
type MockClient struct {
	sync.Mutex
	WatchCh chan Result
	WatchF  func(context.Context) <-chan Result
	Results []mock.Result
	// Delay causes results to be delivered after this period of time has
	// passed. Note that if the context is canceled a result is still consumed
	// from Results.
	Delay time.Duration
	// CloseF is a function to call when the Close function is called on the
	// mock client.
	CloseF func() error
	// if strict rounds is set, calls to get will scan through results to
	// return the first result with the requested round, rather than simply
	// popping the next result and treating it as a stack.
	StrictRounds bool
}

func (m *MockClient) String() string {
	return "Mock"
}

// Get returns a the randomness at `round` or an error.
func (m *MockClient) Get(ctx context.Context, round uint64) (Result, error) {
	m.Lock()
	if len(m.Results) == 0 {
		m.Unlock()
		return nil, errors.New("no result available")
	}
	r := m.Results[0]
	if m.StrictRounds {
		for _, candidate := range m.Results {
			if candidate.Round() == round {
				r = candidate
				break
			}
		}
	} else {
		m.Results = m.Results[1:]
	}
	m.Unlock()

	if m.Delay > 0 {
		t := time.NewTimer(m.Delay)
		select {
		case <-t.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return &r, nil
}

// Watch returns new randomness as it becomes available.
func (m *MockClient) Watch(ctx context.Context) <-chan Result {
	if m.WatchCh != nil {
		return m.WatchCh
	}
	if m.WatchF != nil {
		return m.WatchF(ctx)
	}
	ch := make(chan Result, 1)
	r, err := m.Get(ctx, 0)
	if err == nil {
		ch <- r
	}
	close(ch)
	return ch
}

func (m *MockClient) Info(ctx context.Context) (*chain.Info, error) {
	return nil, errors.New("not supported (mock client info)")
}

// RoundAt will return the most recent round of randomness
func (m *MockClient) RoundAt(_ time.Time) uint64 {
	return 0
}

// Close calls the optional CloseF function.
func (m *MockClient) Close() error {
	if m.CloseF != nil {
		return m.CloseF()
	}
	return nil
}

// ClientWithResults returns a client on which `Get` works `m-n` times.
func MockClientWithResults(n, m uint64) *MockClient {
	c := new(MockClient)
	for i := n; i < m; i++ {
		c.Results = append(c.Results, mock.NewMockResult(i))
	}
	return c
}

// MockClientWithInfo makes a client that returns the given info but no randomness
func MockClientWithInfo(info *chain.Info) Client {
	return &MockInfoClient{info}
}

type MockInfoClient struct {
	i *chain.Info
}

func (m *MockInfoClient) String() string {
	return "MockInfo"
}

func (m *MockInfoClient) Info(ctx context.Context) (*chain.Info, error) {
	return m.i, nil
}

func (m *MockInfoClient) RoundAt(t time.Time) uint64 {
	return chain.CurrentRound(t.Unix(), m.i.Period, m.i.GenesisTime)
}

func (m *MockInfoClient) Get(ctx context.Context, round uint64) (Result, error) {
	return nil, errors.New("not supported (mock info client get)")
}

func (m *MockInfoClient) Watch(ctx context.Context) <-chan Result {
	ch := make(chan Result, 1)
	close(ch)
	return ch
}

func (m *MockInfoClient) Close() error {
	return nil
}

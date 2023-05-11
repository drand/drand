package mock

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/drand/drand/client/test/result/mock"
	chain2 "github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/client"
	"github.com/drand/drand/internal/chain"
)

// Client provide a mocked client interface
//
//nolint:gocritic
type Client struct {
	sync.Mutex
	OptionalInfo *chain2.Info
	WatchCh      chan client.Result
	WatchF       func(context.Context) <-chan client.Result
	Results      []mock.Result
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

func (m *Client) String() string {
	return "Mock"
}

// Get returns a the randomness at `round` or an error.
func (m *Client) Get(ctx context.Context, round uint64) (client.Result, error) {
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
func (m *Client) Watch(ctx context.Context) <-chan client.Result {
	if m.WatchCh != nil {
		return m.WatchCh
	}
	if m.WatchF != nil {
		return m.WatchF(ctx)
	}
	ch := make(chan client.Result, 1)
	r, err := m.Get(ctx, 0)
	if err == nil {
		ch <- r
	}
	close(ch)
	return ch
}

func (m *Client) Info(_ context.Context) (*chain2.Info, error) {
	if m.OptionalInfo != nil {
		return m.OptionalInfo, nil
	}
	return nil, errors.New("not supported (mock client info)")
}

// RoundAt will return the most recent round of randomness
func (m *Client) RoundAt(_ time.Time) uint64 {
	return 0
}

// Close calls the optional CloseF function.
func (m *Client) Close() error {
	if m.CloseF != nil {
		return m.CloseF()
	}
	return nil
}

// ClientWithResults returns a client on which `Get` works `m-n` times.
func ClientWithResults(n, m uint64) *Client {
	c := new(Client)
	for i := n; i < m; i++ {
		c.Results = append(c.Results, mock.NewMockResult(i))
	}
	return c
}

// ClientWithInfo makes a client that returns the given info but no randomness
func ClientWithInfo(info *chain2.Info) client.Client {
	return &InfoClient{info}
}

type InfoClient struct {
	i *chain2.Info
}

func (m *InfoClient) String() string {
	return "MockInfo"
}

func (m *InfoClient) Info(_ context.Context) (*chain2.Info, error) {
	return m.i, nil
}

func (m *InfoClient) RoundAt(t time.Time) uint64 {
	return chain.CurrentRound(t.Unix(), m.i.Period, m.i.GenesisTime)
}

func (m *InfoClient) Get(_ context.Context, _ uint64) (client.Result, error) {
	return nil, errors.New("not supported (mock info client get)")
}

func (m *InfoClient) Watch(_ context.Context) <-chan client.Result {
	ch := make(chan client.Result, 1)
	close(ch)
	return ch
}

func (m *InfoClient) Close() error {
	return nil
}

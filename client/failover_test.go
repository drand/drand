package client

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/test"
)

func TestFailover(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := []MockResult{
		{Rnd: 1, Rand: []byte{1}}, // Success
		{Rnd: 2, Rand: []byte{2}}, // Failover
		{Rnd: 3, Rand: []byte{3}}, // Failover
		{Rnd: 4, Rand: []byte{4}}, // Success
	}

	failC := make(chan Result, 1)
	mockClient := &MockClient{WatchCh: failC, Results: results[1:3]}
	i := fakeChainInfo()
	i.Period = 500 * time.Millisecond
	failoverClient, _ := NewFailoverWatcher(mockClient, i, time.Millisecond*50)
	watchC := failoverClient.Watch(ctx)

	failC <- &results[0]
	compareResults(t, nextResult(t, watchC), &results[0]) // Normal operation
	compareResults(t, nextResult(t, watchC), &results[1]) // First fail
	compareResults(t, nextResult(t, watchC), &results[2]) // Second fail
	failC <- &results[3]
	compareResults(t, nextResult(t, watchC), &results[3]) // Resume normal operattion
}

func TestFailoverDedupe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := []MockResult{
		{Rnd: 1, Rand: []byte{1}}, // Success
		{Rnd: 2, Rand: []byte{2}}, // Failover
		{Rnd: 2, Rand: []byte{2}}, // Success but duplicate
		{Rnd: 3, Rand: []byte{3}}, // Success
	}

	failC := make(chan Result, 2)
	mockClient := &MockClient{WatchCh: failC, Results: results[1:2]}
	i := fakeChainInfo()
	i.Period = 500 * time.Millisecond
	failoverClient, _ := NewFailoverWatcher(mockClient, i, time.Millisecond*50)
	watchC := failoverClient.Watch(ctx)

	failC <- &results[0]
	compareResults(t, nextResult(t, watchC), &results[0]) // Normal operation
	compareResults(t, nextResult(t, watchC), &results[1]) // Failover

	// Two sends but only 1 write to watchC
	failC <- &results[2]
	failC <- &results[3]

	compareResults(t, nextResult(t, watchC), &results[3]) // Success deduped previous
}

func TestFailoverDefaultGrace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := []MockResult{{Rnd: 1, Rand: []byte{1}}}
	failC := make(chan Result)
	mockClient := &MockClient{WatchCh: failC, Results: results}
	i := fakeChainInfo()
	i.Period = 500 * time.Millisecond
	failoverClient, _ := NewFailoverWatcher(mockClient, i, 0)
	watchC := failoverClient.Watch(ctx)

	compareResults(t, nextResult(t, watchC), &results[0])
}

func TestFailoverMaxGrace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := []MockResult{{Rnd: 1, Rand: []byte{1}}}
	failC := make(chan Result)
	mockClient := &MockClient{WatchCh: failC, Results: results}
	period := defaultFailoverGracePeriod / 2
	chainInfo := &chain.Info{
		Period:      period,
		GenesisTime: time.Now().Unix() - 1,
		PublicKey:   test.GenerateIDs(1)[0].Public.Key,
	}
	failoverClient, _ := NewFailoverWatcher(mockClient, chainInfo, 0)
	watchC := failoverClient.Watch(ctx)

	now := time.Now()
	// Should failover in ~period and _definitely_ within gracePeriod!

	r := <-watchC
	compareResults(t, r, &results[0])

	if time.Since(now) >= defaultFailoverGracePeriod {
		t.Fatal("grace period was not bounded to half group period")
	}
}

// errOnGetClient sends it's error to an error channel when Get is called.
type errOnGetClient struct {
	MockClient
	err  error
	errC chan error
}

func (c *errOnGetClient) Get(ctx context.Context, round uint64) (Result, error) {
	c.errC <- c.err
	return nil, c.err
}

func TestFailoverGetFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := []MockResult{
		{Rnd: 1, Rand: []byte{1}},
		{Rnd: 2, Rand: []byte{2}},
	}

	failC := make(chan Result, 1)
	getErr := fmt.Errorf("client get error")
	getErrC := make(chan error, 1)

	mockClient := &errOnGetClient{MockClient: MockClient{WatchCh: failC}, errC: getErrC, err: getErr}

	failoverClient, _ := NewFailoverWatcher(mockClient, fakeChainInfo(), time.Millisecond*50)
	watchC := failoverClient.Watch(ctx)

	failC <- &results[0]
	compareResults(t, nextResult(t, watchC), &results[0]) // Normal operation

	// Wait for error from failover to Get

	if err := <-getErrC; err != getErr {
		t.Fatal("expected error from failover to Get")
	}

	// Write another result and ensure we recover
	failC <- &results[1]
	compareResults(t, nextResult(t, watchC), &results[1])
}

func TestFailoverMissingChainInfo(t *testing.T) {
	mockClient := &MockClient{}
	_, err := NewFailoverWatcher(mockClient, nil, 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "missing chain info" {
		t.Fatal("unexpected error", err)
	}
}

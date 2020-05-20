package client

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
)

func compareResult(t *testing.T, a, b Result) {
	if a.Round() != b.Round() {
		t.Fatal("unexpected result round", a.Round(), b.Round())
	}
	if bytes.Compare(a.Randomness(), b.Randomness()) != 0 {
		t.Fatal("unexpected result randomness", a.Randomness(), b.Randomness())
	}
}

func TestFailover(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := []MockResult{
		{rnd: 1, rand: []byte{1}}, // Success
		{rnd: 2, rand: []byte{2}}, // Failover
		{rnd: 3, rand: []byte{3}}, // Failover
		{rnd: 4, rand: []byte{4}}, // Success
	}

	failC := make(chan Result, 1)
	failC <- &results[0]

	mockClient := &MockClient{WatchCh: failC, Results: results[1:3]}
	group := &key.Group{Period: time.Second, GenesisTime: time.Now().Unix()}
	failoverClient := NewFailoverWatcher(mockClient, group, time.Millisecond*50, log.DefaultLogger)
	watchC := failoverClient.Watch(ctx)

	r0, ok := <-watchC // Normal operation
	if !ok {
		t.Fatal("closed without result")
	}
	compareResult(t, r0, &results[0])

	r1, ok := <-watchC // First fail
	if !ok {
		t.Fatal("closed without result")
	}
	compareResult(t, r1, &results[1])

	r2, ok := <-watchC // Second fail
	if !ok {
		t.Fatal("closed without result")
	}
	compareResult(t, r2, &results[2])

	failC <- &results[3]

	r3, ok := <-watchC // Resume normal operattion
	if !ok {
		t.Fatal("closed without result")
	}
	compareResult(t, r3, &results[3])
}

func TestFailoverDedupe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := []MockResult{
		{rnd: 1, rand: []byte{1}}, // Success
		{rnd: 2, rand: []byte{2}}, // Failover
		{rnd: 2, rand: []byte{2}}, // Success but duplicate
		{rnd: 3, rand: []byte{3}}, // Success
	}

	failC := make(chan Result, 2)
	failC <- &results[0]

	mockClient := &MockClient{WatchCh: failC, Results: results[1:2]}
	group := &key.Group{Period: time.Second, GenesisTime: time.Now().Unix()}
	failoverClient := NewFailoverWatcher(mockClient, group, time.Millisecond*50, log.DefaultLogger)
	watchC := failoverClient.Watch(ctx)

	r0, ok := <-watchC // Normal operation
	if !ok {
		t.Fatal("closed without result")
	}
	compareResult(t, r0, &results[0])

	r1, ok := <-watchC // Failover
	if !ok {
		t.Fatal("closed without result")
	}
	compareResult(t, r1, &results[1])

	// Two sends but only 1 write to watchC
	failC <- &results[2]
	failC <- &results[3]

	r2, ok := <-watchC // Success but duplicate
	if !ok {
		t.Fatal("closed without result")
	}
	compareResult(t, r2, &results[3])
}

func TestFailoverDefaultGrace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := []MockResult{{rnd: 1, rand: []byte{1}}}
	failC := make(chan Result)
	mockClient := &MockClient{WatchCh: failC, Results: results}
	group := &key.Group{Period: time.Second * 10, GenesisTime: time.Now().Unix() - 9}
	failoverClient := NewFailoverWatcher(mockClient, group, 0, log.DefaultLogger)
	watchC := failoverClient.Watch(ctx)

	r0, ok := <-watchC
	if !ok {
		t.Fatal("closed without result")
	}
	compareResult(t, r0, &results[0])
}

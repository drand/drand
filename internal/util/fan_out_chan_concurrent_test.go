package util

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drain reads everything currently buffered in ch without blocking and returns
// the values. It does not wait for more values.
func drainBuffered[T any](ch chan T) []T {
	var out []T
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, v)
		default:
			return out
		}
	}
}

func TestFanOutChan_StopListening(t *testing.T) {
	f := NewFanOutChan[int]()
	defer f.Close()

	l1 := f.Listen()
	l2 := f.Listen()

	f.StopListening(l1)

	// l1 must be closed after StopListening.
	_, ok := <-l1
	assert.False(t, ok, "stopped listener channel must be closed")

	// l2 still receives.
	require.True(t, f.Send(42))
	v, ok := <-l2
	require.True(t, ok)
	assert.Equal(t, 42, v)
}

func TestFanOutChan_StopListeningUnknownChanIsNoop(t *testing.T) {
	f := NewFanOutChan[int]()
	defer f.Close()

	l1 := f.Listen()
	foreign := make(chan int)

	// Removing a channel that was never registered must not panic, not close
	// l1, and not close foreign.
	f.StopListening(foreign)

	require.True(t, f.Send(7))
	v := <-l1
	assert.Equal(t, 7, v)

	// foreign untouched (still open): a non-blocking receive sees no value.
	select {
	case <-foreign:
		t.Fatal("foreign channel should be untouched")
	default:
	}
}

func TestFanOutChan_CloseIsIdempotent(t *testing.T) {
	f := NewFanOutChan[int]()
	l := f.Listen()
	f.Close()
	// second Close must not panic (would double-close listener channels).
	assert.NotPanics(t, func() { f.Close() })
	_, ok := <-l
	assert.False(t, ok)
}

// TestFanOutChan_AllListenersReceive verifies every listener gets every message
// when consumers keep up, using a barrier instead of sleeps.
func TestFanOutChan_AllListenersReceive(t *testing.T) {
	const numListeners = 8
	const numMsgs = MaxMsgsInFlight // stay within buffer so nothing is dropped

	f := NewFanOutChan[int]()
	defer f.Close()

	listeners := make([]chan int, numListeners)
	for i := range listeners {
		listeners[i] = f.Listen()
	}

	for i := 0; i < numMsgs; i++ {
		require.True(t, f.Send(i))
	}

	for li, l := range listeners {
		got := drainBuffered(l)
		require.Len(t, got, numMsgs, "listener %d missed messages", li)
		for i := 0; i < numMsgs; i++ {
			assert.Equal(t, i, got[i], "listener %d out of order", li)
		}
	}
}

// TestFanOutChan_ConcurrentStopWhileSending hammers Send from one goroutine
// while listeners are concurrently added and removed. Run under -race.
func TestFanOutChan_ConcurrentStopWhileSending(t *testing.T) {
	f := NewFanOutChan[int]()

	stop := make(chan struct{})

	// Producer: send continuously until told to stop. It is tracked by its own
	// WaitGroup so that we can stop it *after* the churners are done.
	var producerWg sync.WaitGroup
	var sent atomic.Int64
	producerWg.Add(1)
	go func() {
		defer producerWg.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
				if f.Send(i) {
					sent.Add(1)
				}
				i++
			}
		}
	}()

	// Several churning listeners that constantly Listen/StopListening and drain.
	const churners = 6
	var churnWg sync.WaitGroup
	for c := 0; c < churners; c++ {
		churnWg.Add(1)
		go func() {
			defer churnWg.Done()
			for j := 0; j < 200; j++ {
				ch := f.Listen()
				// drain whatever is there, then stop listening
				drainBuffered(ch)
				f.StopListening(ch)
			}
		}()
	}

	// Wait for all churners to finish, then stop the producer and wait for it.
	churnWg.Wait()
	close(stop)
	producerWg.Wait()

	// nothing to assert beyond "no race / no panic / no deadlock"; ensure we
	// actually exercised Send.
	assert.Positive(t, sent.Load())
	f.Close()
}

// TestFanOutChan_NoGoroutineLeak ensures Listen/StopListening/Close don't spawn
// lingering goroutines (the implementation shouldn't, but guard against it).
func TestFanOutChan_NoGoroutineLeak(t *testing.T) {
	runtime.GC()
	before := runtime.NumGoroutine()

	for i := 0; i < 50; i++ {
		f := NewFanOutChan[int]()
		l1 := f.Listen()
		l2 := f.Listen()
		f.Send(i)
		drainBuffered(l1)
		f.StopListening(l1)
		f.Close()
		drainBuffered(l2)
	}

	runtime.GC()
	after := runtime.NumGoroutine()
	// allow a small slack for the test runtime/scheduler.
	assert.LessOrEqual(t, after, before+2, "goroutines leaked: before=%d after=%d", before, after)
}

// TestFanOutChan_ConcurrentSenders documents behavior: the type is documented
// as single-producer, but Send uses an RLock so concurrent Sends must at least
// be race-free. This guards that invariant under -race.
func TestFanOutChan_ConcurrentSenders(t *testing.T) {
	f := NewFanOutChan[int]()
	defer f.Close()
	l := f.Listen()

	var wg sync.WaitGroup
	for s := 0; s < 4; s++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				f.Send(i)
				drainBuffered(l) // keep buffer from overflowing
			}
		}()
	}
	wg.Wait()
}

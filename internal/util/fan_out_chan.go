package util

import (
	"sync"
)

// MaxDKGsInFlight is an arbitrary limit set for the number of DKGs in flight to avoid
// overallocating channel capacity for the fanout channel
const MaxDKGsInFlight = 20

// FanOutChan has one producer channel and multiple consumers for each message on the channel
type FanOutChan[T any] struct {
	lock      sync.RWMutex
	delegate  chan T
	listeners []chan T
	closed    bool
}

func NewFanOutChan[T any]() *FanOutChan[T] {
	f := &FanOutChan[T]{
		delegate:  make(chan T, MaxDKGsInFlight),
		listeners: make([]chan T, 0),
	}

	go func() {
		for item := range f.delegate {
			f.lock.RLock()
			for _, l := range f.listeners {
				// non blocking send
				// keep RLock to avoid send vs close races.
				select {
				case l <- item:
				default:
				}
			}
			f.lock.RUnlock()
		}
	}()

	return f
}

func (f *FanOutChan[T]) Listen() chan T {
	ch := make(chan T, MaxDKGsInFlight)

	f.lock.Lock()
	f.listeners = append(f.listeners, ch)
	f.lock.Unlock()
	return ch
}

func (f *FanOutChan[T]) StopListening(ch chan T) {
	f.lock.Lock()
	defer f.lock.Unlock()
	for i, l := range f.listeners {
		if l == ch {
			f.listeners = append(f.listeners[0:i], f.listeners[i+1:]...)
			close(ch)
			break
		}
	}
}

// Send sends an item to the delegate channel while holding the read lock,
// preventing a race with Close(). Returns false if the channel is closed.
func (f *FanOutChan[T]) Send(item T) (sent bool) {
	f.lock.RLock()
	if f.closed {
		f.lock.RUnlock()

		return false
	}
	f.lock.RUnlock()

	f.delegate <- item
	return true
}

func (f *FanOutChan[T]) Close() {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.closed {
		return
	}
	f.closed = true
	close(f.delegate)
	for _, l := range f.listeners {
		close(l)
	}
	f.listeners = make([]chan T, 0)
}

package util

import (
	"sync"
)

// MaxMsgsInFlight is an arbitrary limit set for the number of messages in flight to avoid
// overallocating channel capacity for the fanout channel
const MaxMsgsInFlight = 20

// FanOutChan has one producer and multiple consumers: each message sent is
// delivered to every listener via a non-blocking send.
type FanOutChan[T any] struct {
	lock      sync.RWMutex
	listeners []chan T
	closed    bool
}

func NewFanOutChan[T any]() *FanOutChan[T] {
	return &FanOutChan[T]{
		listeners: make([]chan T, 0),
	}
}

func (f *FanOutChan[T]) Listen() chan T {
	ch := make(chan T, MaxMsgsInFlight)

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

// Send fans out item to all listeners using non-blocking sends.
// Returns false if the channel has been closed.
func (f *FanOutChan[T]) Send(item T) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()
	if f.closed {
		return false
	}
	for _, l := range f.listeners {
		select {
		case l <- item:
		default:
		}
	}
	return true
}

func (f *FanOutChan[T]) Close() {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.closed {
		return
	}
	f.closed = true
	for _, l := range f.listeners {
		close(l)
	}
	f.listeners = make([]chan T, 0)
}

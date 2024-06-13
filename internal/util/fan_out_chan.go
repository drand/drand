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
				l := l
				l <- item
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
	defer f.lock.Unlock()
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

func (f *FanOutChan[T]) Chan() chan T {
	return f.delegate
}

func (f *FanOutChan[T]) Close() {
	f.lock.Lock()
	defer f.lock.Unlock()
	for _, l := range f.listeners {
		close(l)
	}
	f.listeners = make([]chan T, 0)
}

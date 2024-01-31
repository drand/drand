package util

import (
	"sync"
)

const MaxDKGsInFlight = 20

// FanOutChan has one producer channel and multiple consumers for each message ont he channel
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

func (f *FanOutChan[T]) Chan() chan T {
	return f.delegate
}

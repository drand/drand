package util

import (
	"testing"
	"time"
)

func TestFanOutChan_NonBlockingSend(t *testing.T) {
	f := NewFanOutChan[int]()
	defer f.Close()

	// Create a listener with small buffer
	listener := f.Listen()

	// Fill the listener channel to capacity
	for i := 0; i < MaxDKGsInFlight; i++ {
		listener <- i
	}

	// Send a message - should not block even though listener is full
	done := make(chan bool)
	go func() {
		f.Chan() <- 999
		done <- true
	}()

	select {
	case <-done:
		// Success - send completed without blocking
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Send operation blocked when listener channel was full")
	}
}

func TestFanOutChan_CloseStopsGoroutine(t *testing.T) {
	f := NewFanOutChan[int]()

	// Create a listener
	listener := f.Listen()

	// Send a message before closing
	f.Chan() <- 1

	// Close the fanout - this should close delegate and all listeners
	f.Close()

	// Verify delegate channel is closed (send should panic)
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("Expected panic when sending to closed delegate channel")
			}
		}()
		f.Chan() <- 2
	}()

	// Verify listener is closed
	select {
	case _, ok := <-listener:
		if ok {
			t.Fatal("Listener channel should be closed")
		}
		// Channel is closed, which is expected
	default:
		t.Fatal("Listener channel should be closed and empty")
	}
}

func TestFanOutChan_MultipleListeners(t *testing.T) {
	f := NewFanOutChan[string]()
	defer f.Close()

	listener1 := f.Listen()
	listener2 := f.Listen()

	// Send a message
	f.Chan() <- "test"

	// Both listeners should receive it
	select {
	case msg := <-listener1:
		if msg != "test" {
			t.Fatalf("Expected 'test', got %q", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Listener1 did not receive message")
	}

	select {
	case msg := <-listener2:
		if msg != "test" {
			t.Fatalf("Expected 'test', got %q", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Listener2 did not receive message")
	}
}

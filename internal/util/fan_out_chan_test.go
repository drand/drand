package util

import (
	"testing"
	"time"
)

func TestFanOutChan_NonBlockingSend(t *testing.T) {
	f := NewFanOutChan[int]()
	defer f.Close()

	listener := f.Listen()

	// Fill the listener channel to capacity
	for i := 0; i < MaxMsgsInFlight; i++ {
		listener <- i
	}

	// Send a message - should not block even though listener is full
	done := make(chan bool)
	go func() {
		f.Send(999)
		done <- true
	}()

	select {
	case <-done:
		// Success - send completed without blocking
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Send operation blocked when listener channel was full")
	}
}

func TestFanOutChan_CloseAndSend(t *testing.T) {
	f := NewFanOutChan[int]()

	listener := f.Listen()

	if !f.Send(1) {
		t.Fatal("Send should succeed before Close")
	}

	f.Close()

	if f.Send(2) {
		t.Fatal("Send should fail after Close")
	}

	// First receive should yield the value sent before Close
	select {
	case num, ok := <-listener:
		if !ok {
			t.Fatal("expected to receive the buffered value")
		}
		if num != 1 {
			t.Fatalf("expected 1, got %d", num)
		}
	default:
		t.Fatal("expected a buffered value")
	}

	// Second receive should see a closed channel
	select {
	case _, ok := <-listener:
		if ok {
			t.Fatal("listener channel should be closed")
		}
	default:
		t.Fatal("listener channel should be closed, not empty")
	}
}

func TestFanOutChan_SendRespectsClosedChan(t *testing.T) {
	f := NewFanOutChan[int]()

	// Close the fanout - this should close delegate and all listeners
	f.Close()

	if f.Send(1) {
		t.Fatal("Listener channel should be closed and Send should not send")
	}
}

func TestFanOutChan_MultipleListeners(t *testing.T) {
	f := NewFanOutChan[string]()
	defer f.Close()

	listener1 := f.Listen()
	listener2 := f.Listen()

	// Send a message
	f.Send("test")

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

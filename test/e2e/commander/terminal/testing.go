package terminal

import (
	"testing"
	"time"
)

// TestingTerminal is a wrapper aorund a terminal to make testing easier.
type TestingTerminal struct {
	Terminal *Terminal
}

// ForTesting creates a new testing terminal session for running
// commands. It makes testing easier by adding a *testing.T parameter to
// methods so that the caller does not have to check for errors.
func ForTesting(id string) *TestingTerminal {
	return &TestingTerminal{Terminal: New(id)}
}

// Run executes the passed command in the terminal but it does not wait for it
// to complete, it panics if another command is already running.
func (tt *TestingTerminal) Run(t *testing.T, name string, args ...string) {
	err := tt.Terminal.Run(name, args...)
	if err != nil {
		t.Fatal(err)
	}
}

// Kill terminates the currently running command. It blocks until the command is
// observed to exit and calls t.Fatal if it takes longer than 1 minute. If the
// current command is no longer running this is a noop.
func (tt *TestingTerminal) Kill(t *testing.T) {
	err := tt.Terminal.Kill()
	if err != nil {
		t.Fatal(err)
	}
}

// KillWithTimeout terminates the currently running command. It blocks until the
// command is observed to exit and calls t.Fatal if it takes longer than the
// passed timeout duration. If the current command is no longer running this is
// a noop.
func (tt *TestingTerminal) KillWithTimeout(t *testing.T, timeout time.Duration) {
	err := tt.Terminal.KillWithTimeout(timeout)
	if err != nil {
		t.Fatal(err)
	}
}

// AwaitSuccess blocks until the current command completes without error for the
// default timeout of 1 minute.
func (tt *TestingTerminal) AwaitSuccess(t *testing.T) {
	err := tt.Terminal.AwaitSuccess()
	if err != nil {
		t.Fatal(err)
	}
}

// AwaitSuccessWithTimeout blocks until the current command completes without
// error for up to the passed amount of time - it then calls t.Fatal.
func (tt *TestingTerminal) AwaitSuccessWithTimeout(t *testing.T, timeout time.Duration) {
	err := tt.Terminal.AwaitSuccessWithTimeout(timeout)
	if err != nil {
		t.Fatal(err)
	}
}

// AwaitOutput blocks until the current command stdout contains the passed
// substr or it calls t.Fatal if it takes longer than 1 minute.
func (tt *TestingTerminal) AwaitOutput(t *testing.T, substr string) string {
	match, err := tt.Terminal.AwaitOutput(substr)
	if err != nil {
		t.Fatal(err)
	}
	return match
}

// AwaitOutputWithTimeout blocks until the current command stdout contains the
// passed substr or it calls t.Fatal if the timeout is reached.
func (tt *TestingTerminal) AwaitOutputWithTimeout(t *testing.T, substr string, timeout time.Duration) string {
	match, err := tt.Terminal.AwaitOutputWithTimeout(substr, timeout)
	if err != nil {
		t.Fatal(err)
	}
	return match
}

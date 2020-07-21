package commander

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/drand/drand/test/e2e/commander/io"
)

// Terminal is a place were commands run.
type Terminal struct {
	sync.Mutex
	id      string
	command *Command
}

// NewTerminal creates a new terminal session for running commands.
func NewTerminal(id string) *Terminal {
	return &Terminal{id: id}
}

// Run runs the passed command in the terminal, it panics if another command is
// already running.
func (term *Terminal) Run(t *testing.T, name string, args ...string) {
	term.Lock()
	defer term.Unlock()

	var running bool
	if term.command != nil {
		select {
		case <-term.command.Done():
		default:
			running = true
		}
	}
	if running {
		t.Fatal(fmt.Errorf("running command: another command is already running: \"%s\"", term.command.String()))
	}
	// redirect command stdout/err
	stdout := io.NewMultiWriter(io.PrefixedWriter(term.id, os.Stdout))
	stderr := io.PrefixedWriter(term.id, os.Stderr)
	term.command = NewCommand(name, args, stdout, stderr)

	err := term.command.Run()
	if err != nil {
		t.Fatal(fmt.Errorf("running command: %w", err))
	}
}

// Cancel terminates the currently running command. If the current command is no
// longer running this is a noop.
func (term *Terminal) Cancel() {
	term.Lock()
	defer term.Unlock()
	if term.command != nil {
		term.command.Cancel()
	}
}

// AwaitSuccess blocks until the current command completes without error for the
// default timeout of 1 minute.
func (term *Terminal) AwaitSuccess(t *testing.T) {
	term.AwaitSuccessWithTimeout(t, time.Minute)
}

// AwaitSuccessWithTimeout blocks until the current command completes without
// error for up to the passed amount of time - it then panics
func (term *Terminal) AwaitSuccessWithTimeout(t *testing.T, timeout time.Duration) {
	term.Lock()
	defer term.Unlock()

	timer := time.NewTimer(timeout)
	select {
	case <-term.command.Done():
		timer.Stop()
	case <-timer.C:
		t.Fatal(fmt.Errorf("timed out waiting for command to complete"))
	}

	if term.command.Err() != nil {
		t.Fatal(fmt.Errorf("command exited with error: %w", term.command.Err()))
	}
}

// AwaitOutput blocks until the current command stdout contains the passed
// substr or it will panic if it takes longer than 1 minute.
func (term *Terminal) AwaitOutput(t *testing.T, substr string) string {
	return term.AwaitOutputWithTimeout(t, substr, time.Minute)
}

// AwaitOutputWithTimeout blocks until the current command stdout contains the
// passed substr or it will panic if the timeout is reached.
func (term *Terminal) AwaitOutputWithTimeout(t *testing.T, substr string, timeout time.Duration) string {
	term.Lock()
	defer term.Unlock()

	matcher := io.NewMatchingWriter(substr)
	mw, ok := term.command.stdout.(*io.MultiWriter)
	if !ok {
		t.Fatal("stdout was not a MultiWriter")
	}
	mw.Add(matcher)
	defer mw.Remove(matcher)

	timer := time.NewTimer(timeout)
	select {
	case msg := <-matcher.C:
		timer.Stop()
		return msg
	case <-term.command.Done():
		timer.Stop()
		t.Fatal(fmt.Errorf("command completed without matching \"%s\"", substr))
	case <-timer.C:
		t.Fatal(fmt.Errorf("timed out waiting for output matching \"%s\"", substr))
	}
	return ""
}

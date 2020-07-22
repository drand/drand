package commander

import (
	"fmt"
	"os"
	"sync"
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

// Run executes the passed command in the terminal but it does not wait for it
// to complete, it panics if another command is already running.
func (term *Terminal) Run(name string, args ...string) error {
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
		return fmt.Errorf("another command is already running: \"%s\"", term.command.String())
	}
	// redirect command stdout/err
	stdout := io.NewMultiWriter(io.PrefixedWriter(term.id, os.Stdout))
	stderr := io.PrefixedWriter(term.id, os.Stderr)
	term.command = NewCommand(name, args, stdout, stderr)

	err := term.command.Run()
	if err != nil {
		return fmt.Errorf("running command: %w", err)
	}
	return nil
}

// Kill terminates the currently running command. It blocks until the command is
// observed to exit and returns an error if it takes longer than 1 minute. If
// the current command is no longer running this is a noop.
func (term *Terminal) Kill() error {
	return term.KillWithTimeout(DefaultTimeout)
}

// KillWithTimeout terminates the currently running command. It blocks until the
// command is observed to exit and returns an error if it takes longer than the
// passed timeout duration. If the current command is no longer running this is
// a noop.
func (term *Terminal) KillWithTimeout(timeout time.Duration) error {
	term.Lock()
	defer term.Unlock()

	if term.command == nil {
		return nil
	}

	term.command.Cancel()
	timer := time.NewTimer(timeout)
	select {
	case <-term.command.Done():
		timer.Stop()
	case <-timer.C:
		return fmt.Errorf("timed out waiting for killed command to exit")
	}

	if term.command.Err() == nil || term.command.Err().Error() != "signal: killed" {
		return fmt.Errorf("unexpected exit error: %w", term.command.Err())
	}

	return nil
}

// AwaitSuccess blocks until the current command completes without error for the
// default timeout of 1 minute.
func (term *Terminal) AwaitSuccess() error {
	return term.AwaitSuccessWithTimeout(DefaultTimeout)
}

// AwaitSuccessWithTimeout blocks until the current command completes without
// error for up to the passed amount of time, then it returns an error.
func (term *Terminal) AwaitSuccessWithTimeout(timeout time.Duration) error {
	term.Lock()
	defer term.Unlock()

	timer := time.NewTimer(timeout)
	select {
	case <-term.command.Done():
		timer.Stop()
	case <-timer.C:
		return fmt.Errorf("timed out waiting for command to complete")
	}

	if term.command.Err() != nil {
		return fmt.Errorf("command exited with error: %w", term.command.Err())
	}

	return nil
}

// AwaitOutput blocks until the current command stdout contains the passed
// substr or it returns an error if it takes longer than 1 minute.
func (term *Terminal) AwaitOutput(substr string) (string, error) {
	return term.AwaitOutputWithTimeout(substr, DefaultTimeout)
}

// AwaitOutputWithTimeout blocks until the current command stdout contains the
// passed substr or it returns an error if the timeout is reached.
func (term *Terminal) AwaitOutputWithTimeout(substr string, timeout time.Duration) (string, error) {
	term.Lock()
	defer term.Unlock()

	matcher := io.NewMatchingWriter(substr)
	mw, ok := term.command.stdout.(*io.MultiWriter)
	if !ok {
		return "", fmt.Errorf("stdout was not a MultiWriter")
	}
	mw.Add(matcher)
	defer mw.Remove(matcher)

	timer := time.NewTimer(timeout)
	select {
	case msg := <-matcher.C:
		timer.Stop()
		return msg, nil
	case <-term.command.Done():
		timer.Stop()
		return "", fmt.Errorf("command completed without matching \"%s\"", substr)
	case <-timer.C:
		return "", fmt.Errorf("timed out waiting for output matching \"%s\"", substr)
	}
}

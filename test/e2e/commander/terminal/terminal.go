package terminal

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/drand/drand/test/e2e/commander/command"
	"github.com/drand/drand/test/e2e/commander/io"
)

// DefaultTimeout is the timeout used for actions when a timeout is not
// specified. If you need a specific timeout, you can usually use a
// `xxxWithTimeout` variant instead.
const DefaultTimeout = time.Minute

// Terminal is a place were commands run.
type Terminal struct {
	sync.Mutex
	id      string
	command *command.Command
}

// New creates a new terminal session for running commands.
func New(id string) *Terminal {
	return &Terminal{id: id}
}

// Run executes the passed command in the terminal but it does not wait for it
// to complete, it panics if another command is already running.
func (t *Terminal) Run(name string, args ...string) error {
	t.Lock()
	defer t.Unlock()

	var running bool
	if t.command != nil {
		select {
		case <-t.command.Done():
		default:
			running = true
		}
	}
	if running {
		return fmt.Errorf("another command is already running: \"%s\"", t.command.String())
	}
	// redirect command stdout/err
	stdout := io.NewMultiWriter(io.PrefixedWriter(t.id, os.Stdout))
	stderr := io.PrefixedWriter(t.id, os.Stderr)
	t.command = command.New(name, args, stdout, stderr)

	err := t.command.Run()
	if err != nil {
		return fmt.Errorf("running command: %w", err)
	}
	return nil
}

// Kill terminates the currently running command. It blocks until the command is
// observed to exit and returns an error if it takes longer than 1 minute. If
// the current command is no longer running this is a noop.
func (t *Terminal) Kill() error {
	return t.KillWithTimeout(DefaultTimeout)
}

// KillWithTimeout terminates the currently running command. It blocks until the
// command is observed to exit and returns an error if it takes longer than the
// passed timeout duration. If the current command is no longer running this is
// a noop.
func (t *Terminal) KillWithTimeout(timeout time.Duration) error {
	t.Lock()
	defer t.Unlock()

	if t.command == nil {
		return nil
	}

	t.command.Cancel()
	timer := time.NewTimer(timeout)
	select {
	case <-t.command.Done():
		timer.Stop()
	case <-timer.C:
		return fmt.Errorf("timed out waiting for killed command to exit")
	}

	if t.command.Err() == nil || t.command.Err().Error() != "signal: killed" {
		return fmt.Errorf("unexpected exit error: %w", t.command.Err())
	}

	return nil
}

// AwaitSuccess blocks until the current command completes without error for the
// default timeout of 1 minute.
func (t *Terminal) AwaitSuccess() error {
	return t.AwaitSuccessWithTimeout(DefaultTimeout)
}

// AwaitSuccessWithTimeout blocks until the current command completes without
// error for up to the passed amount of time, then it returns an error.
func (t *Terminal) AwaitSuccessWithTimeout(timeout time.Duration) error {
	t.Lock()
	defer t.Unlock()

	timer := time.NewTimer(timeout)
	select {
	case <-t.command.Done():
		timer.Stop()
	case <-timer.C:
		return fmt.Errorf("timed out waiting for command to complete")
	}

	if t.command.Err() != nil {
		return fmt.Errorf("command exited with error: %w", t.command.Err())
	}

	return nil
}

// AwaitOutput blocks until the current command stdout contains the passed
// substr or it returns an error if it takes longer than 1 minute.
func (t *Terminal) AwaitOutput(substr string) (string, error) {
	return t.AwaitOutputWithTimeout(substr, DefaultTimeout)
}

// AwaitOutputWithTimeout blocks until the current command stdout contains the
// passed substr or it returns an error if the timeout is reached.
func (t *Terminal) AwaitOutputWithTimeout(substr string, timeout time.Duration) (string, error) {
	t.Lock()
	defer t.Unlock()

	matcher := io.NewMatchingWriter(substr)
	mw, ok := t.command.Stdout().(*io.MultiWriter)
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
	case <-t.command.Done():
		timer.Stop()
		return "", fmt.Errorf("command completed without matching \"%s\"", substr)
	case <-timer.C:
		return "", fmt.Errorf("timed out waiting for output matching \"%s\"", substr)
	}
}

package manager

import (
	"sync"
	"time"

	"github.com/drand/drand/test/e2e/commander/terminal"
)

// TerminalManager allows for management of multiple terminals.
type TerminalManager struct {
	terms []*terminal.Terminal
}

// New creates a new TerminalManager instance.
func New(terms ...*terminal.Terminal) *TerminalManager {
	return &TerminalManager{terms}
}

// AwaitSuccess blocks until all current commands in the terminals complete
// without error for the default timeout of 1 minute.
func (tm *TerminalManager) AwaitSuccess() error {
	return tm.AwaitSuccessWithTimeout(terminal.DefaultTimeout)
}

// AwaitSuccessWithTimeout blocks until all current commands in the terminals
// complete without error for up to the passed amount of time, then it returns
// an error.
func (tm *TerminalManager) AwaitSuccessWithTimeout(timeout time.Duration) error {
	wg := sync.WaitGroup{}
	wg.Add(len(tm.terms))

	lk := sync.Mutex{}
	var err error // TODO: multierror

	for _, t := range tm.terms {
		go func(t *terminal.Terminal) {
			successErr := t.AwaitSuccessWithTimeout(timeout)
			if successErr != nil {
				lk.Lock()
				err = successErr
				lk.Unlock()
			}
			wg.Done()
		}(t)
	}

	wg.Wait()
	return err
}

// AwaitOutput blocks until all current command's stdout contains the passed
// substr or it returns an error if it takes longer than 1 minute.
func (tm *TerminalManager) AwaitOutput(substr string) ([]string, error) {
	return tm.AwaitOutputWithTimeout(substr, terminal.DefaultTimeout)
}

// AwaitOutputWithTimeout blocks until all current command's stdout contains the
// passed substr or it returns an error if the timeout is reached.
func (tm *TerminalManager) AwaitOutputWithTimeout(substr string, timeout time.Duration) ([]string, error) {
	matches := make([]string, len(tm.terms))

	wg := sync.WaitGroup{}
	wg.Add(len(tm.terms))

	lk := sync.Mutex{}
	var err error // TODO: multierror

	for i, t := range tm.terms {
		go func(i int, t *terminal.Terminal) {
			match, outErr := t.AwaitOutputWithTimeout(substr, timeout)
			if outErr != nil {
				lk.Lock()
				err = outErr
				lk.Unlock()
			}
			matches[i] = match
			wg.Done()
		}(i, t)
	}

	wg.Wait()

	return matches, err
}

// Kill terminates all current terminal commands. It blocks until the commands
// are observed to exit and returns an error if it takes longer than 1 minute.
// If the current command is no longer running this is a noop.
func (tm *TerminalManager) Kill() error {
	return tm.KillWithTimeout(terminal.DefaultTimeout)
}

// KillWithTimeout terminates the currently running command. It blocks until the
// command is observed to exit and returns an error if it takes longer than the
// passed timeout duration. If the current command is no longer running this is
// a noop.
func (tm *TerminalManager) KillWithTimeout(timeout time.Duration) error {
	wg := sync.WaitGroup{}
	wg.Add(len(tm.terms))

	lk := sync.Mutex{}
	var err error // TODO: multierror

	for _, t := range tm.terms {
		go func(t *terminal.Terminal) {
			killErr := t.KillWithTimeout(timeout)
			if killErr != nil {
				lk.Lock()
				err = killErr
				lk.Unlock()
			}
			wg.Done()
		}(t)
	}

	wg.Wait()
	return err
}

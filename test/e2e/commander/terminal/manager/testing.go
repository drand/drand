package manager

import (
	"testing"
	"time"

	"github.com/drand/drand/test/e2e/commander/terminal"
)

// TestingTerminalManager is a terminal manager what makes testing easier.
type TestingTerminalManager struct {
	TerminalManager *TerminalManager
}

// ForTesting creates a TestingTerminalManager for the passed terminals.
func ForTesting(testingTerms ...*terminal.TestingTerminal) *TestingTerminalManager {
	var terms []*terminal.Terminal
	for _, t := range testingTerms {
		terms = append(terms, t.Terminal)
	}
	return &TestingTerminalManager{TerminalManager: New(terms...)}
}

// AwaitSuccess blocks until all current commands in the terminals complete
// without error for the default timeout of 1 minute.
func (ttm *TestingTerminalManager) AwaitSuccess(t *testing.T) {
	err := ttm.TerminalManager.AwaitSuccess()
	if err != nil {
		t.Fatal(err)
	}
}

// AwaitSuccessWithTimeout blocks until all current commands in the terminals
// complete without error for up to the passed amount of time, then it calls
// t.Fatal.
func (ttm *TestingTerminalManager) AwaitSuccessWithTimeout(t *testing.T, timeout time.Duration) {
	err := ttm.TerminalManager.AwaitSuccessWithTimeout(timeout)
	if err != nil {
		t.Fatal(err)
	}
}

// AwaitOutput blocks until all current command's stdout contains the passed
// substr or it calls t.Fatal if it takes longer than 1 minute.
func (ttm *TestingTerminalManager) AwaitOutput(t *testing.T, substr string) []string {
	matches, err := ttm.TerminalManager.AwaitOutput(substr)
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

// AwaitOutputWithTimeout blocks until all current command's stdout contains the
// passed substr or it calls t.Fatal if the timeout is reached.
func (ttm *TestingTerminalManager) AwaitOutputWithTimeout(t *testing.T, substr string, timeout time.Duration) []string {
	matches, err := ttm.TerminalManager.AwaitOutputWithTimeout(substr, timeout)
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

// Kill terminates all current terminal commands. It blocks until the commands
// are observed to exit and calls t.Fatal if it takes longer than 1 minute.
func (ttm *TestingTerminalManager) Kill(t *testing.T) {
	err := ttm.TerminalManager.Kill()
	if err != nil {
		t.Fatal(err)
	}
}

// KillWithTimeout terminates all current terminal commands. It blocks until the
// commands are observed to exit and calls t.Fatal if it takes longer than the
// passed timeout duration.
func (ttm *TestingTerminalManager) KillWithTimeout(t *testing.T, timeout time.Duration) {
	err := ttm.TerminalManager.KillWithTimeout(timeout)
	if err != nil {
		t.Fatal(err)
	}
}

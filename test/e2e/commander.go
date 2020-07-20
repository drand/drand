package e2e

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// Terminal is a place were commands run
type Terminal struct {
	sync.Mutex
	command *Command
}

// NewTerminal creates a new terminal session for running commands.
func NewTerminal() *Terminal {
	return &Terminal{}
}

// Command returns the current command (which may or may not still be running).
func (t *Terminal) Command() *Command {
	return t.command
}

// Run runs the passed command in the terminal, it panics if another command is
// already running.
func (t *Terminal) Run(cmd string) {
	t.Lock()
	defer t.Unlock()

	var running bool
	select {
	case <-t.command.Done():
	default:
		running = true
	}
	if running {
		panic(fmt.Errorf("running command, another command is already running: %s", t.command.Cmd))
	}
	t.command = NewCommand(cmd)
	t.command.Run()
}

// Cancel terminates the currently running command. If the current command is no
// longer running this is a noop.
func (t *Terminal) Cancel() {
	t.Lock()
	defer t.Unlock()
	if t.command != nil {
		t.command.Cancel()
	}
}

// Wait blocks until the current command finishes for up to the passed amount of
// time - it then panics
func (t *Terminal) Wait(timeout time.Duration) {
	t.Lock()
	defer t.Unlock()
	timer := time.NewTimer(timeout)
	select {
	case <-t.command.Done():
		timer.Stop()
	case <-timer.C:
		panic(fmt.Errorf("timed out waiting for command to complete: %s", t.command.Cmd))
	}
}

// Command encapsulates a command that is running or did run in a terminal.
type Command struct {
	sync.RWMutex
	Cmd    string
	err    error
	stdout string
	stderr string
	done   chan struct{}
	cancel context.CancelFunc
}

// NewCommand creates a new command instance.
func NewCommand(cmd string) *Command {
	return &Command{
		Cmd:  cmd,
		done: make(chan struct{}),
	}
}

// Run runs the command.
func (c *Command) Run() {
	c.Lock()
	defer c.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	cmd := exec.CommandContext(ctx, c.Cmd)
	go func() {
		err := cmd.Wait()
		c.Lock()
		c.err = err
		close(c.done)
		c.Unlock()
	}()
	cmd.Start()
}

// Stdout returns the command's accumulated stdout messages as a string.
func (c *Command) Stdout() string {
	c.RLock()
	defer c.RUnlock()
	return c.stdout
}

// Stderr returns the command's accumulated stderr messages as a string.
func (c *Command) Stderr() string {
	c.RLock()
	defer c.RUnlock()
	return c.stderr
}

// Done returns a channel that closes when the command completes.
func (c *Command) Done() chan struct{} {
	return c.done
}

// Cancel will kill the command if it is running.
func (c *Command) Cancel() {
	c.RLock()
	defer c.RUnlock()
	if c.cancel != nil {
		c.cancel()
	}
}

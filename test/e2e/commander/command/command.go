package command

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Command encapsulates a command run in a terminal.
type Command struct {
	sync.RWMutex
	name   string
	args   []string
	err    error
	stdout io.Writer
	stderr io.Writer
	done   chan struct{}
	cancel context.CancelFunc
}

// New creates a new command instance.
func New(name string, args []string, stdout, stderr io.Writer) *Command {
	return &Command{
		name:   name,
		args:   args,
		stdout: stdout,
		stderr: stderr,
		done:   make(chan struct{}),
	}
}

// String returns a string representation of the running command
func (c *Command) String() string {
	return fmt.Sprintf("%s %s", c.name, strings.Join(c.args, " "))
}

// Run runs the command but does not wait for it to complete.
func (c *Command) Run() error {
	c.Lock()
	defer c.Unlock()

	bin, err := exec.LookPath(c.name)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	cmd := exec.CommandContext(ctx, bin, c.args...)
	fmt.Fprintf(c.stdout, "%s\n", c.String())

	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr

	err = cmd.Start()
	if err != nil {
		return err
	}

	go func() {
		err := cmd.Wait()
		c.Lock()
		c.err = err
		c.Unlock()
		close(c.done)
	}()

	return nil
}

// Stdout returns the stdout writer for this command.
func (c *Command) Stdout() io.Writer {
	return c.stdout
}

// Stderr returns the stderr writer for this command.
func (c *Command) Stderr() io.Writer {
	return c.stderr
}

// Err returns the command exit error if there was one.
func (c *Command) Err() error {
	c.RLock()
	defer c.RUnlock()
	return c.err
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

package commander

import (
	"testing"
	"time"
)

// DefaultTimeout is the timeout used for actions when a timeout is not
// specified. If you need a specific timeout, you can usually use a
// `xxxWithTimeout` variant instead.
const DefaultTimeout = time.Minute

// AwaitCommandSuccess waits for the success of multiple commands
func AwaitCommandSuccess(t *testing.T, commands ...Command) {

}

func AwaitCommandSuccessWithTimeout(t *testing.T, commands ...Command) {

}

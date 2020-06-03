package basic

import (
	"github.com/drand/drand/log"
)

// LoggingClient sets the logger for use by clients that suppport it
type LoggingClient interface {
	SetLog(log.Logger)
}

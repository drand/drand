package client

import (
	clientinterface "github.com/drand/drand/client/interface"
	"github.com/drand/drand/log"
)

// Client represents the drand Client interface.
type Client = clientinterface.Client

// LoggingClient sets the logger for use by clients that suppport it
type LoggingClient interface {
	SetLog(log.Logger)
}

// Result represents the randomness for a single drand round.
type Result = clientinterface.Result

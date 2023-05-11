package testlogger

import (
	"os"
	"testing"

	"github.com/drand/drand/common/log"
)

// Level returns the level to default the logger based on the DRAND_TEST_LOGS presence
func Level(t *testing.T) int {
	logLevel := log.InfoLevel
	debugEnv, isDebug := os.LookupEnv("DRAND_TEST_LOGS")
	if isDebug && debugEnv == "DEBUG" {
		t.Log("Enabling DebugLevel logs")
		logLevel = log.DebugLevel
	}

	return logLevel
}

// New returns a configured logger
func New(t *testing.T) log.Logger {
	return log.New(nil, Level(t), true).
		With("testName", t.Name())
}

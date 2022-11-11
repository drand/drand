package test

import (
	"os"
	"testing"

	"github.com/drand/drand/log"
)

// LogLevel returns the level to default the logger based on the DRAND_TEST_LOGS presence
func LogLevel(t *testing.T) int {
	logLevel := log.LogInfo
	debugEnv, isDebug := os.LookupEnv("DRAND_TEST_LOGS")
	if isDebug && debugEnv == "DEBUG" {
		t.Log("Enabling LogDebug logs")
		logLevel = log.LogDebug
	}

	return logLevel
}

// Logger returns a configured logger
func Logger(t *testing.T) log.Logger {
	return log.NewLogger(nil, LogLevel(t))
}

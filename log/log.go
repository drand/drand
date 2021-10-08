package log

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is a interface that can log to different levels.
type Logger interface {
	Info(keyvals ...interface{})
	Debug(keyvals ...interface{})
	Warn(keyvals ...interface{})
	Error(keyvals ...interface{})
	Fatal(keyvals ...interface{})
	Panic(keyvals ...interface{})
	Infow(msg string, keyvals ...interface{})
	Debugw(msg string, keyvals ...interface{})
	Warnw(msg string, keyvals ...interface{})
	Errorw(msg string, keyvals ...interface{})
	Fatalw(msg string, keyvals ...interface{})
	Panicw(msg string, keyvals ...interface{})
	With(args ...interface{}) *zap.SugaredLogger
}

const (
	LogInfo  = int(zapcore.InfoLevel)
	LogDebug = int(zapcore.DebugLevel)
	LogError = int(zapcore.ErrorLevel)
	LogFatal = int(zapcore.FatalLevel)
	LogPanic = int(zapcore.PanicLevel)
	LogWarn  = int(zapcore.WarnLevel)
)

// DefaultLevel is the default level where statements are logged. Change the
// value of this variable before init() to change the level of the default
// logger.
const DefaultLevel = LogInfo

var defaultLoggerSet sync.Once

// SetDefaultLogger updates the default logger to wrap a provided kit logger.
func SetDefaultLogger(output zapcore.WriteSyncer, level int) {
	if output == nil {
		output = os.Stdout
	}

	zap.ReplaceGlobals(NewZapLogger(output, level))
}

// DefaultLogger is the default logger that only logs at the `DefaultLevel`.
func DefaultLogger() Logger {
	defaultLoggerSet.Do(func() {
		SetDefaultLogger(nil, DefaultLevel)
	})

	return zap.S()
}

// NewLogger returns a kit logger that prints statements at the given level.
func NewLogger(output zapcore.WriteSyncer, level int) Logger {
	logger := NewZapLogger(output, level)

	return logger.Sugar()
}

func NewZapLogger(output zapcore.WriteSyncer, level int) *zap.Logger {
	if output == nil {
		output = os.Stdout
	}

	encoderConfig := zap.NewProductionEncoderConfig()

	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	encoder := zapcore.NewConsoleEncoder(encoderConfig)

	core := zapcore.NewCore(encoder, output, zapcore.Level(level))

	logger := zap.New(core, zap.WithCaller(true))

	return logger
}

package log

import (
	"context"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// log is the implementation of Logger
type log struct {
	*zap.SugaredLogger
}

// Logger is an interface that can log to different levels.
//
//nolint:interfacebloat // We want this interface to implement the original one
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
	With(args ...interface{}) Logger
	Named(s string) Logger
	AddCallerSkip(skip int) Logger
}

func (l *log) AddCallerSkip(skip int) Logger {
	return &log{l.WithOptions(zap.AddCallerSkip(skip))}
}

func (l *log) With(args ...interface{}) Logger {
	return &log{l.SugaredLogger.With(args...)}
}

func (l *log) Named(s string) Logger {
	return &log{l.SugaredLogger.Named(s)}
}

const (
	InfoLevel  = int(zapcore.InfoLevel)
	DebugLevel = int(zapcore.DebugLevel)
	ErrorLevel = int(zapcore.ErrorLevel)
	FatalLevel = int(zapcore.FatalLevel)
	PanicLevel = int(zapcore.PanicLevel)
	WarnLevel  = int(zapcore.WarnLevel)
)

// DefaultLevel is the default level where statements are logged. Change the
// value of this variable before init() to change the level of the default
// logger.
var DefaultLevel = InfoLevel

// Allows the debug logs to be printed in envs where the test logs are set to debug level.
//
//nolint:gochecknoinits // We do want to overwrite the default log level here
func init() {
	debugEnv, isDebug := os.LookupEnv("DRAND_TEST_LOGS")
	if isDebug && debugEnv == "DEBUG" {
		DefaultLevel = DebugLevel
	}
}

var isDefaultLoggerSet sync.Once

// ConfigureDefaultLogger updates the default logger to wrap a provided kit logger.
func ConfigureDefaultLogger(output zapcore.WriteSyncer, level int, jsonFormat bool) {
	encoder := getConsoleEncoder()
	if jsonFormat {
		encoder = getJSONEncoder()
	}
	zap.ReplaceGlobals(newZapLogger(output, encoder, level))
}

// DefaultLogger is the default logger that only logs at the `DefaultLevel`.
func DefaultLogger() Logger {
	isDefaultLoggerSet.Do(func() {
		zap.ReplaceGlobals(newZapLogger(nil, getJSONEncoder(), DefaultLevel))
	})

	return &log{zap.S()}
}

// New returns a logger that prints statements at the given level.
func New(output zapcore.WriteSyncer, level int, isJSON bool) Logger {
	encoder := getConsoleEncoder()
	if isJSON {
		encoder = getJSONEncoder()
	}
	l := newZapLogger(output, encoder, level)
	return &log{l.Sugar()}
}

func newZapLogger(output zapcore.WriteSyncer, encoder zapcore.Encoder, level int) *zap.Logger {
	if output == nil {
		output = os.Stdout
	}

	core := zapcore.NewCore(encoder, output, zapcore.Level(level))
	logger := zap.New(core, zap.WithCaller(true))
	return logger
}

func getJSONEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()

	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	return zapcore.NewJSONEncoder(encoderConfig)
}

func getConsoleEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()

	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	return zapcore.NewConsoleEncoder(encoderConfig)
}

type ctxLoggerKey string

const ctxLogger ctxLoggerKey = "drandLogger"

// ToContext allows setting the logger on the context
func ToContext(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, ctxLogger, l)
}

// FromContextOrDefault returns the logger from the context when set with CtxWithLogger.
// If not found, it returns a nil value.
func FromContextOrDefault(ctx context.Context) Logger {
	l, ok := ctx.Value(ctxLogger).(Logger)
	if !ok {
		l = DefaultLogger()
		l.Debugw("logger missing on context, using default logger")
	}
	return l
}

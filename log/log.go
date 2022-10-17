package log

import (
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type logger struct {
	*zap.SugaredLogger
}

// Logger is a interface that can log to different levels.
//
//nolint
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
}

func (l *logger) With(args ...interface{}) Logger {
	return &logger{l.SugaredLogger.With(args...)}
}

func (l *logger) Named(s string) Logger {
	return &logger{l.SugaredLogger.Named(s)}
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

var isDefaultLoggerSet sync.Once

// ConfigureDefaultLogger updates the default logger to wrap a provided kit logger.
func ConfigureDefaultLogger(output zapcore.WriteSyncer, level int, jsonFormat bool) {
	if jsonFormat {
		zap.ReplaceGlobals(NewZapLogger(output, getJSONEncoder(), level))
	} else {
		zap.ReplaceGlobals(NewZapLogger(output, getConsoleEncoder(), level))
	}
}

// DefaultLogger is the default logger that only logs at the `DefaultLevel`.
func DefaultLogger() Logger {
	isDefaultLoggerSet.Do(func() {
		zap.ReplaceGlobals(NewZapLogger(nil, getConsoleEncoder(), DefaultLevel))
	})

	return &logger{zap.S()}
}

// NewLogger returns a logger that prints statements at the given level.
func NewLogger(output zapcore.WriteSyncer, level int) Logger {
	l := NewZapLogger(output, getConsoleEncoder(), level)
	return &logger{l.Sugar()}
}

// NewJSONLogger returns a logger that prints statements at the given level as JSON output.
func NewJSONLogger(output zapcore.WriteSyncer, level int) Logger {
	l := NewZapLogger(output, getJSONEncoder(), level)
	return &logger{l.Sugar()}
}

func NewZapLogger(output zapcore.WriteSyncer, encoder zapcore.Encoder, level int) *zap.Logger {
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

type fixedTime time.Time

func (f fixedTime) Now() time.Time {
	return time.Time(f)
}

func (f fixedTime) NewTicker(time.Duration) *time.Ticker {
	return &time.Ticker{}
}

// FixedTimeLogger sets the logging time to a specific time, making it easier to compare
// the output between runs.
func FixedTimeLogger(l Logger) Logger {
	lg := l.(*logger)
	timeValue := time.Date(2022, 10, 17, 0, 0, 0, 0, time.UTC)
	return &logger{lg.WithOptions(zap.WithClock(fixedTime(timeValue)))}
}

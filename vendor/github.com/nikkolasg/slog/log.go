package slog

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

type LogLevel int

const (
	levelFatal = iota
	LevelPrint
	LevelInfo
	LevelDebug
)

var Level LogLevel

var Output io.Writer

func init() {
	Output = os.Stdout
	Level = LevelPrint
}

var (
	/* PrintPrefix = "[+]"*/
	//InfoPrefix  = "[i]"
	//DebugPrefix = "DEBUG"
	/*FatalPrefix = "FATAL"*/
	PrintPrefix = ""
	InfoPrefix  = ""
	DebugPrefix = ""
	FatalPrefix = ""
)

func p(lvl LogLevel, args ...interface{}) {
	if lvl > Level {
		return
	}
	var buff bytes.Buffer
	switch lvl {
	case LevelPrint:
		buff.WriteString(PrintPrefix)
	case LevelInfo:
		buff.WriteString(InfoPrefix)
	case LevelDebug:
		buff.WriteString(DebugPrefix)
	case levelFatal:
		buff.WriteString(FatalPrefix)
	default:
		panic("wrong slog level")
	}
	buff.WriteString(fmt.Sprintln(args...))
	fmt.Fprintf(Output, buff.String())
}

func ErrFatal(err error) {
	if err == nil {
		return
	}
	p(levelFatal, err.Error())
	os.Exit(-1)
}

func ErrFatalf(err error, str string, args ...interface{}) {
	if err == nil {
		return
	}
	p(levelFatal, fmt.Sprintf(str))
	os.Exit(-1)
}

func Fatal(args ...interface{}) {
	p(levelFatal, args...)
	os.Exit(-1)
}

func Fatalf(str string, args ...interface{}) {
	p(levelFatal, fmt.Sprintf(str, args...))
	os.Exit(-1)
}

func Print(args ...interface{}) {
	p(LevelPrint, args...)
}

func Printf(str string, args ...interface{}) {
	p(LevelPrint, fmt.Sprintf(str, args...))
}

func Info(args ...interface{}) {
	p(LevelInfo, args...)
}

func Infof(str string, args ...interface{}) {
	p(LevelInfo, fmt.Sprintf(str, args...))
}

func Debug(args ...interface{}) {
	p(LevelDebug, args...)
}

func Debugf(str string, args ...interface{}) {
	p(LevelDebug, fmt.Sprintf(str, args...))
}

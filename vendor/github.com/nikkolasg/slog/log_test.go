package slog

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogs(t *testing.T) {
	var buff = new(bytes.Buffer)
	Output = buff
	Level = LevelDebug
	defer func() {
		Output = os.Stdout
		Level = LevelPrint
	}()

	text := "hello"
	for _, fn := range []func(...interface{}){Print, Info, Debug} {
		fn(text)
		assert.Contains(t, buff.String(), text)
		buff = new(bytes.Buffer)
		Output = buff
	}
	textf := "hello number %d"
	id := 5
	result := fmt.Sprintf(textf, id)
	for _, fn := range []func(string, ...interface{}){Printf, Infof, Debugf} {
		fn(textf, id)
		assert.Contains(t, buff.String(), result)
		buff = new(bytes.Buffer)
		Output = buff

	}
}

func TestLevels(t *testing.T) {
	buff := new(bytes.Buffer)
	Output = buff
	defer func() { Output = os.Stdout }()

	reset := func() {
		buff = new(bytes.Buffer)
		Output = buff
	}

	text := "hello"
	Level = LevelPrint
	Info(text)
	assert.NotContains(t, buff.String(), text)
	Infof(text)
	assert.NotContains(t, buff.String(), text)

	Level = LevelInfo
	Info(text)
	assert.Contains(t, buff.String(), text)

	reset()
	Infof(text)
	assert.Contains(t, buff.String(), text)

	reset()
	debug := "debug"
	Debug(debug)
	assert.NotContains(t, buff.String(), debug)

	reset()
	Level = LevelDebug
	Debug(debug)
	assert.Contains(t, buff.String(), debug)
	reset()

	Debugf(debug)
	assert.Contains(t, buff.String(), debug)
}

package log

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestLoggerKit(t *testing.T) {
	type logTest struct {
		with       []interface{}
		level      int
		allowedLvl int
		msg        string
		out        []string
	}

	w := func(kv ...interface{}) []interface{} {
		return kv
	}
	o := func(outs ...string) []string {
		return outs
	}
	var tests = []logTest{
		{nil, InfoLevel, InfoLevel, "hello", o("hello")},
		{nil, DebugLevel, InfoLevel, "hello", nil},
		{nil, ErrorLevel, DebugLevel, "hello", o("hello")},
		{nil, WarnLevel, ErrorLevel, "hello", nil},
		{nil, WarnLevel, DebugLevel, "hello", o("hello")},
		{w("yard", "bird"), WarnLevel, InfoLevel, "hello", o("yard", "bird", "hello")},
	}

	for i, test := range tests {
		t.Logf(" -- test %d -- \n", i)

		var b bytes.Buffer
		writer := bufio.NewWriter(&b)
		syncer := zapcore.AddSync(writer)

		var logging func(...interface{})
		logger := New(syncer, test.allowedLvl, true)

		if test.with != nil {
			logger = logger.With(test.with...)
		}

		switch test.level {
		case InfoLevel:
			logging = logger.Info
		case DebugLevel:
			logging = logger.Debug
		case WarnLevel:
			logging = logger.Warn
		case ErrorLevel:
			logging = logger.Error
		case FatalLevel:
			logging = logger.Fatal
		case PanicLevel:
			logging = logger.Panic
		default:
			t.FailNow()
		}

		logging("msg=", test.msg)
		writer.Flush()

		if test.out != nil {
			requireContains(t, &b, test.out, true)
		} else {
			requireContains(t, &b, nil, false)
		}
	}
}

func TestOddKV(t *testing.T) {
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	syncer := zapcore.AddSync(writer)

	logger := New(syncer, InfoLevel, true)
	logger = logger.With([]interface{}{"yard", "bird", "stone"}...)

	logger.Info("msg=", "hello")
	writer.Flush()

	out := b.String()

	require.Contains(t, out, "msg=hello")
	require.Contains(t, out, "Ignored key without a value.")
}

func requireContains(t *testing.T, r io.Reader, outs []string, present bool) {
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	if !present {
		require.Equal(t, string(out), "")
		return
	}
	for _, o := range outs {
		require.Contains(t, string(out), o)
	}
	require.NotContains(t, string(out), "Ignored key without a value.")
}
